package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	ptypes "github.com/golang/protobuf/ptypes"
	"github.com/sprawl/sprawl/config"
	"github.com/sprawl/sprawl/database/leveldb"
	"github.com/sprawl/sprawl/errors"
	"github.com/sprawl/sprawl/identity"
	"github.com/sprawl/sprawl/interfaces"
	"github.com/sprawl/sprawl/p2p"
	"github.com/sprawl/sprawl/pb"
	"github.com/sprawl/sprawl/util"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"google.golang.org/grpc"
	bufconn "google.golang.org/grpc/test/bufconn"
)

const testConfigPath string = "../config/test"
const dbPathVar string = "database.path"
const websocketPortVar string = "websocket.port"
const dialContext string = "TestEndpoint"
const asset1 string = "ETH"
const asset2 string = "BTC"
const assetPair string = "BTC,ETH"
const testAmount = 52617562718
const testPrice = 0.1

var bufSize = 1024 * 1024
var lis *bufconn.Listener
var conn *grpc.ClientConn
var err error
var ctx context.Context
var storage *leveldb.Storage = &leveldb.Storage{}
var p2pInstance *p2p.P2p
var websocketService *WebsocketService
var testConfig *config.Config
var s *grpc.Server
var orderClient pb.OrderHandlerClient
var orderService interfaces.OrderService = &OrderService{Logger: new(util.PlaceholderLogger)}
var channelService interfaces.ChannelService = &ChannelService{}
var channel *pb.Channel
var logger *zap.Logger
var log *zap.SugaredLogger

func init() {
	logger = zap.NewNop()
	log = logger.Sugar()
	testConfig = &config.Config{}
	privateKey, publicKey, _ := identity.GenerateKeyPair(rand.Reader)
	p2pInstance = p2p.NewP2p(testConfig, privateKey, publicKey, p2p.Logger(log))
	testConfig.ReadConfig(testConfigPath)
	storage.SetDbPath(testConfig.GetDatabasePath())
	websocketService = &WebsocketService{Logger: log, Port: testConfig.GetWebsocketPort()}
}

func createNewServerInstance() {
	p2pInstance.Run()
	storage.Run()

	ctx = context.Background()
	lis = bufconn.Listen(bufSize)

	conn, err = grpc.DialContext(ctx, dialContext, grpc.WithDialer(BufDialer), grpc.WithInsecure())
	if !errors.IsEmpty(err) {
		panic(err)
	}

	s = grpc.NewServer()

	orderClient = pb.NewOrderHandlerClient(conn)

	// Register services
	channelService.RegisterStorage(storage)
	channelService.RegisterP2p(p2pInstance)
}

func joinTestChannel(t *testing.T) {
	joinres, err := channelService.Join(ctx, &pb.JoinRequest{Asset: asset1, CounterAsset: asset2})
	assert.NoError(t, err)
	channel = joinres.GetJoinedChannel()
}

func removeAllOrders() {
	storage.DeleteAllWithPrefix(string(interfaces.OrderPrefix))
}

func BufDialer(string, time.Duration) (net.Conn, error) {
	return lis.Dial()
}

func TestOrderStorageKeyPrefixer(t *testing.T) {
	prefixedBytes := getOrderStorageKey([]byte(assetPair), []byte(asset1))
	assert.Equal(t, string(prefixedBytes), string(interfaces.OrderPrefix)+string(assetPair)+string(asset1))
}

func TestOrderQueryPrefixer(t *testing.T) {
	prefixedBytes := getOrderQueryPrefix([]byte(assetPair))
	assert.Equal(t, string(prefixedBytes), string(interfaces.OrderPrefix)+string(assetPair))
}

func TestOrderCreation(t *testing.T) {
	createNewServerInstance()
	orderService.RegisterStorage(storage)
	orderService.RegisterP2p(p2pInstance)

	defer p2pInstance.Close()
	defer storage.Close()
	defer conn.Close()
	removeAllOrders()

	testOrder := pb.CreateRequest{ChannelID: channel.GetId(), Asset: asset1, CounterAsset: asset2, Amount: testAmount, Price: testPrice}
	var lastOrder *pb.Order

	// Register order endpoints with the gRPC server
	pb.RegisterOrderHandlerServer(s, orderService)

	go func() {
		if err := s.Serve(lis); !errors.IsEmpty(err) {
			t.Logf("Server exited with error: %v", err)
		}
		defer s.Stop()
	}()

	resp, err := orderClient.Create(ctx, &testOrder)
	assert.NoError(t, err)
	t.Logf("Created Order: %s", resp)
	assert.NotNil(t, resp)

	lastOrder = resp.GetCreatedOrder()
	storedOrder, err := orderClient.GetOrder(ctx, &pb.OrderSpecificRequest{OrderID: lastOrder.GetId(), ChannelID: channel.GetId()})
	assert.NoError(t, err)

	assert.Equal(t, lastOrder, storedOrder)

	resp2, err := orderClient.Delete(ctx, &pb.OrderSpecificRequest{OrderID: lastOrder.GetId(), ChannelID: channel.GetId()})
	assert.NoError(t, err)
	assert.NotNil(t, resp2)
}

func TestOrderReceive(t *testing.T) {
	createNewServerInstance()
	orderService.RegisterStorage(storage)
	orderService.RegisterWebsocket(websocketService)
	defer p2pInstance.Close()
	defer storage.Close()
	defer conn.Close()
	removeAllOrders()

	ws, err := StartServer(websocketService)
	defer websocketService.Close()
	assert.NoError(t, err)

	testOrder := pb.CreateRequest{ChannelID: channel.GetId(), Asset: asset1, CounterAsset: asset2, Amount: testAmount, Price: testPrice}

	// Register order endpoints with the gRPC server
	pb.RegisterOrderHandlerServer(s, orderService)

	go func() {
		if err := s.Serve(lis); !errors.IsEmpty(err) {
			t.Fatalf("Server exited with error: %v", err)
		}
		defer s.Stop()
	}()

	order, err := orderService.Create(ctx, &testOrder)
	marshaledOrder, err := proto.Marshal(order)

	err = orderService.Receive(marshaledOrder, p2pInstance.GetHostID())

	wireMessage := &pb.WireMessage{}

	assert.NoError(t, err)
	err = proto.Unmarshal(marshaledOrder, wireMessage)
	assert.NoError(t, err)

	_, p, err := ws.ReadMessage()
	assert.NoError(t, err)
	testWireMessage2 := &pb.WireMessage{}
	proto.Unmarshal(p, testWireMessage2)
	assert.Equal(t, wireMessage, testWireMessage2)

	storedOrder, err := orderClient.GetOrder(ctx, &pb.OrderSpecificRequest{OrderID: order.GetCreatedOrder().GetId(), ChannelID: channel.GetId()})
	assert.NoError(t, err)
	assert.NotNil(t, storedOrder)
}

func TestSignAndVerifyOrder(t *testing.T) {
	createNewServerInstance()
	orderService.RegisterStorage(storage)
	defer p2pInstance.Close()
	defer storage.Close()
	defer conn.Close()
	removeAllOrders()

	testOrder := pb.CreateRequest{ChannelID: channel.GetId(), Asset: asset1, CounterAsset: asset2, Amount: testAmount, Price: testPrice}
	now := ptypes.TimestampNow()

	// TODO: Use the node's private key here as a secret to sign the Order ID with
	secret := "mysecret"

	// Create a new HMAC by defining the hash type and the key (as byte array)
	h := hmac.New(sha256.New, []byte(secret))

	// Write Data to it
	h.Write(append([]byte(testOrder.String()), []byte(now.String())...))
	// Get result and encode as hexadecimal string
	id := h.Sum(nil)

	// Get current timestamp as protobuf type
	// Construct the order
	order := &pb.Order{
		Id:           id,
		Created:      now,
		Asset:        testOrder.Asset,
		CounterAsset: testOrder.CounterAsset,
		Amount:       testOrder.Amount,
		Price:        testOrder.Price,
		State:        pb.State_LOCKED,
	}

	_, publicKey, err := identity.GetIdentity(storage)

	sig, err := orderService.GetSignature(order)
	assert.NoError(t, err)
	order.Signature = sig
	order.State = pb.State_OPEN
	success, err := orderService.VerifyOrder(publicKey, order)
	assert.NoError(t, err)
	assert.True(t, success)
	order.State = pb.State_LOCKED
	success, err = orderService.VerifyOrder(publicKey, order)
	assert.NoError(t, err)
	assert.True(t, success)
}
func TestOrderGetAll(t *testing.T) {
	createNewServerInstance()
	orderService.RegisterStorage(storage)
	defer p2pInstance.Close()
	defer storage.Close()
	defer conn.Close()
	removeAllOrders()

	testOrder := pb.CreateRequest{ChannelID: channel.GetId(), Asset: asset1, CounterAsset: asset2, Amount: testAmount, Price: testPrice}

	// Register order endpoints with the gRPC server
	pb.RegisterOrderHandlerServer(s, orderService)

	go func() {
		if err := s.Serve(lis); !errors.IsEmpty(err) {
			t.Fatalf("Server exited with error: %v", err)
		}
		defer s.Stop()
	}()

	const testIterations = int(4)
	for i := 0; i < testIterations; i++ {
		_, err := orderClient.Create(ctx, &testOrder)
		assert.True(t, errors.IsEmpty(err))
	}

	resp, err := orderClient.GetAllOrders(ctx, &pb.Empty{})
	assert.True(t, errors.IsEmpty(err))
	orders := resp.GetOrders()
	assert.Equal(t, len(orders), testIterations)
}

func BenchmarkOrderReceive(b *testing.B) {
	createNewServerInstance()
	orderService.RegisterStorage(storage)
	orderService.RegisterP2p(p2pInstance)
	defer p2pInstance.Close()
	defer storage.Close()
	defer conn.Close()
	removeAllOrders()

	testOrder := pb.CreateRequest{ChannelID: channel.GetId(), Asset: asset1, CounterAsset: asset2, Amount: testAmount, Price: testPrice}

	// Register order endpoints with the gRPC server
	pb.RegisterOrderHandlerServer(s, orderService)

	go func() {
		if err := s.Serve(lis); !errors.IsEmpty(err) {
			b.Fatalf("Server exited with error: %v", err)
		}
		defer s.Stop()
	}()

	b.ResetTimer()
	for i := 1; i < b.N; i++ {
		order, _ := orderService.Create(ctx, &testOrder)
		marshaledOrder, _ := proto.Marshal(order)
		orderService.Receive(marshaledOrder, p2pInstance.GetHostID())
		orderClient.GetOrder(ctx, &pb.OrderSpecificRequest{OrderID: order.GetCreatedOrder().GetId()})
	}
}

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	fmt "fmt"

	"github.com/eqlabs/sprawl/interfaces"
	"github.com/eqlabs/sprawl/pb"
	"github.com/golang/protobuf/proto"
	ptypes "github.com/golang/protobuf/ptypes"
)

// OrderService implements the OrderService Server service.proto
type OrderService struct {
	storage interfaces.Storage
	p2p     interfaces.P2p
}

// RegisterStorage registers a storage service to store the Orders in
func (s *OrderService) RegisterStorage(storage interfaces.Storage) {
	s.storage = storage
}

// RegisterP2p registers a p2p service
func (s *OrderService) RegisterP2p(p2p interfaces.P2p) {
	s.p2p = p2p
}

// Create creates an Order, storing it locally and broadcasts the Order to all other nodes on the channel
func (s *OrderService) Create(ctx context.Context, in *pb.CreateRequest) (*pb.CreateResponse, error) {
	// Get current timestamp as protobuf type
	now := ptypes.TimestampNow()

	// TODO: Use the node's private key here as a secret to sign the Order ID with
	secret := "mysecret"

	// Create a new HMAC by defining the hash type and the key (as byte array)
	h := hmac.New(sha256.New, []byte(secret))

	// Write Data to it
	h.Write(append([]byte(in.String()), []byte(now.String())...))

	// Get result and encode as hexadecimal string
	id := h.Sum(nil)
	fmt.Println(s)
	// Construct the order
	order := &pb.Order{
		Id:           id,
		Created:      now,
		Asset:        in.Asset,
		CounterAsset: in.CounterAsset,
		Amount:       in.Amount,
		Price:        in.Price,
		State:        pb.State_OPEN,
	}

	// Get order as bytes
	orderInBytes, err := proto.Marshal(order)
	if err != nil {
		panic(err)
	}

	// Save order to LevelDB locally
	err = s.storage.Put(id, orderInBytes)
	if err != nil {
		panic(err)
	}

	// TODO: Propagate order to other nodes via sprawl/p2p

	// TODO: Properly return any errors to client instead of panicking
	// Return the response to the gRPC client
	return &pb.CreateResponse{
		CreatedOrder: order,
		Error:        nil,
	}, nil
}

// Delete removes the Order with the specified ID locally, and broadcasts the same request to all other nodes on the channel
func (s *OrderService) Delete(ctx context.Context, in *pb.OrderSpecificRequest) (*pb.GenericResponse, error) {
	// Try to delete the Order from LevelDB with specified ID
	err := s.storage.Delete(in.GetId())
	if err != nil {
		panic(err)
	}

	// TODO: Propagate the deletion to other nodes via sprawl/p2p
	// TODO: Properly return any errors to client instead of panicking
	return &pb.GenericResponse{
		Error: nil,
	}, nil
}

// Lock locks the given Order if the Order is created by this node, broadcasts the lock to other nodes on the channel.
func (s *OrderService) Lock(ctx context.Context, in *pb.OrderSpecificRequest) (*pb.GenericResponse, error) {

	// TODO: Add Order locking logic

	return &pb.GenericResponse{
		Error: nil,
	}, nil
}

// Unlock unlocks the given Order if it's created by this node, broadcasts the unlocking operation to other nodes on the channel.
func (s *OrderService) Unlock(ctx context.Context, in *pb.OrderSpecificRequest) (*pb.GenericResponse, error) {

	// TODO: Add Order unlocking logic

	return &pb.GenericResponse{
		Error: nil,
	}, nil
}

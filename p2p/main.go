package p2p

import (
	"bufio"
	"context"
	"fmt"
	"sync"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	network "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	multiaddr "github.com/multiformats/go-multiaddr"
)

type P2p struct {
	config           Config
	ctx              context.Context
	host             host.Host
	kademliaDHT      *dht.IpfsDHT
	routingDiscovery *discovery.RoutingDiscovery
	peerChan         <-chan peer.AddrInfo
}

func handleStream(stream network.Stream) {
	// Create a buffer stream for non blocking read and write.
	reader := bufio.NewReader(stream)

	go readData(reader)

	// 'stream' will stay open until you close it (or the other side closes it).
}

func readData(reader *bufio.Reader) {
	for {
		bytes, err := reader.ReadBytes(byte('\n'))
		if err != nil {
			fmt.Println("Error reading from buffer")
			panic(err)
		}
		if bytes == nil {
			return
		}
		if bytes[0] != byte('\n') {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			fmt.Printf("\x1b[32m%s\x1b[0m> ", bytes)
		}
	}
}

func writeData(writer *bufio.Writer, input []byte) {
	_, err := writer.Write(input)
	if err != nil {
		fmt.Println("Error writing to buffer")
		panic(err)
	}

	err = writer.Flush()
	if err != nil {
		fmt.Println("Error flushing buffer")
		panic(err)
	}
}

func (p2p *P2p) createConfig() {
	var err error
	p2p.config, err = ParseFlags()
	if err != nil {
		panic(err)
	}
}

func (p2p *P2p) createContext() {
	p2p.ctx = context.Background()
}

func (p2p *P2p) createHost() {
	var err error
	p2p.host, err = libp2p.New(p2p.ctx,
		libp2p.ListenAddrs([]multiaddr.Multiaddr(p2p.config.ListenAddresses)...),
	)
	if err != nil {
		panic(err)
	}
}

func (p2p *P2p) createKademliaDHT() {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	var err error
	p2p.kademliaDHT, err = dht.New(p2p.ctx, p2p.host)
	if err != nil {
		panic(err)
	}

}

func (p2p *P2p) bootstrapDHT() {
	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	var err error
	if err = p2p.kademliaDHT.Bootstrap(p2p.ctx); err != nil {
		panic(err)
	}
}

func (p2p *P2p) getPeerAddresses() {
	// Let's connect to the bootstrap nodes first. They will tell us about the
	// other nodes in the network.
	var wg sync.WaitGroup
	for _, peerAddr := range p2p.config.BootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p2p.host.Connect(p2p.ctx, *peerinfo); err != nil {
				fmt.Println(err)
			} else {
				fmt.Println(peerinfo)
			}
		}()
	}
	wg.Wait()
}

func (p2p *P2p) createRoutingDiscovery() {
	p2p.routingDiscovery = discovery.NewRoutingDiscovery(p2p.kademliaDHT)
}

func (p2p *P2p) advertise() {
	discovery.Advertise(p2p.ctx, p2p.routingDiscovery, p2p.config.RendezvousString)
}

func (p2p *P2p) findPeers() {
	var err error
	p2p.peerChan, err = p2p.routingDiscovery.FindPeers(p2p.ctx, p2p.config.RendezvousString)
	if err != nil {
		panic(err)
	}
}

func (p2p *P2p) SendToPeers(input []byte) {
	p2p.sendToPeers(p2p.ctx, p2p.config, p2p.host, p2p.peerChan, input)
}

func (p2p *P2p) sendToPeers(ctx context.Context, config Config, host host.Host, peerChan <-chan peer.AddrInfo, input []byte) {
	for peer := range peerChan {
		if peer.ID == host.ID() {
			continue
		}
		stream, err := host.NewStream(ctx, peer.ID, protocol.ID(config.ProtocolID))

		if err != nil {
			continue
		} else {
			writer := bufio.NewWriter(stream)
			writeData(writer, input)
		}
	}
}

func (p2p *P2p) listenPeers() {
	for peer := range p2p.peerChan {
		if peer.ID == p2p.host.ID() {
			continue
		}
		stream, err := p2p.host.NewStream(p2p.ctx, peer.ID, protocol.ID(p2p.config.ProtocolID))

		if err != nil {
			continue
		} else {
			reader := bufio.NewReader(stream)
			go readData(reader)
		}
	}
}

// Run runs the p2p network
func (p2p *P2p) Run() {
	// Set a function as stream handler. This function is called when a peer
	// initiates a connection and starts a stream with this peer.
	p2p.createConfig()
	p2p.createContext()
	p2p.createHost()
	p2p.createKademliaDHT()
	p2p.host.SetStreamHandler(protocol.ID(p2p.config.ProtocolID), handleStream)
	p2p.bootstrapDHT()
	p2p.getPeerAddresses()
	p2p.createRoutingDiscovery()
	p2p.advertise()
	p2p.findPeers()
	p2p.listenPeers()
	select {}
}

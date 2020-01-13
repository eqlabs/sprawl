package p2p

import (
	"bufio"
	"fmt"

	"github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/sprawl/sprawl/errors"
	"github.com/sprawl/sprawl/interfaces"
)

// Stream is a single streaming connection between two peers
type Stream struct {
	stream network.Stream
	input  *bufio.Writer
	output *bufio.Reader
}

func (p2p *P2p) handleStream(buf network.Stream) {
	if p2p.Logger != nil {
		p2p.Logger.Info("New stream opened")
	}
	reader := bufio.NewReader(bufio.NewReader(buf))
	stream := Stream{stream: buf, output: reader}
	go stream.receiveStream(reader, p2p.Receiver)
}

func (stream *Stream) receiveStream(reader *bufio.Reader, receiver interfaces.Receiver) error {
	for {
		data, err := reader.ReadBytes('\n')
		if err != nil {
			return errors.E(errors.Op("Reading bytes from stream"), err)
		} else {
			if receiver != nil {
				err := receiver.Receive(data)
				if !errors.IsEmpty(err) {
					return errors.E(errors.Op("Passing data from stream to receiver"), err)
				}
			} else {
				return errors.E(errors.Op("No receiver defined for stream.receiveStream"))
			}
		}
		if string(data) == "" {
			stream.stream.Close()
			return nil
		}
	}
}

func (stream *Stream) writeToStream(data []byte) error {
	_, err := stream.input.Write(data)
	err = stream.input.Flush()
	return err
}

// OpenStream opens a stream with another Sprawl peer
func (p2p *P2p) OpenStream(peerIDString string) error {
	peerID, err := peer.IDFromString(peerIDString)
	fmt.Println("SPRAWL", peerID, err, peerIDString, networkID)
	stream, err := p2p.host.NewStream(p2p.ctx, peerID, networkID)
	if err != nil {
		p2p.Logger.Errorf("Stream open failed: %s", err)
	} else {
		writer := bufio.NewWriter(bufio.NewWriter(stream))
		p2p.streams[peerIDString] = Stream{stream: stream, input: writer}
		p2p.Logger.Debugf("Stream opened with %s", peerID)
	}
	return err
}

// CloseStream removes and closes a stream
func (p2p *P2p) CloseStream(peerIDString string) error {
	err := p2p.streams[peerIDString].stream.Close()
	delete(p2p.streams, peerIDString)
	return err
}

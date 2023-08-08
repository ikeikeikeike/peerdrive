package main

import (
	"context"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/core/host"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/rjeczalik/notify"
)

const syncProtocol = "/peerdrive/1.0.0"

func main() {
	h, err := libp2p.New()
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	log.Println("Host created. We are:", h.ID())

	h.SetStreamHandler(syncProtocol, handleStream)

	c := make(chan notify.EventInfo, 1)

	if err := notify.Watch(".", c, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	for {
		select {
		case event := <-c:
			log.Printf("Received event: %v\n", event)
			sendFileChange(h, event)
		}
	}
}

func sendFileChange(h host.Host, event notify.EventInfo) {
	targetAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmOtherPeerID")
	targetInfo, _ := peer.AddrInfoFromP2pAddr(targetAddr)

	stream, err := h.NewStream(context.Background(), targetInfo.ID, syncProtocol)
	if err != nil {
		log.Println("Error opening stream:", err)
		return
	}

	_, err = stream.Write([]byte(event.Event().String()))
	if err != nil {
		log.Println("Error sending file change event:", err)
		return
	}
}

func handleStream(s network.Stream) {
	buf := make([]byte, 1024)
	n, _ := s.Read(buf)
	log.Printf("Received file change event: %s\n", buf[:n])
}

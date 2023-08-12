package main

import (
	"log"

	"github.com/ikeikeikeike/peerdrive/p2p"
	"github.com/ikeikeikeike/peerdrive/sync"
)

func main() {
	// Arguments
	args, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}

	// P2P Host
	h, err := p2p.NewP2P(args.Port)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// For local-network
	// dMDNS, err := p2p.NewMDNS(h, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// For global-network
	dDHT, err := p2p.NewDHT(h, args.Rendezvous)
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Printf("Peer: %s\n", h.ID())

	// Discover
	// go dMDNS.Run( )
	go dDHT.Run()

	// Packet
	h.SetStreamHandler(sync.SyncProtocol, sync.SyncHandler(dDHT.DHT()))

	// synchornize
	sync.SyncWatcher(dDHT.DHT(), args.SyncDir)
}

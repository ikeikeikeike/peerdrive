package main

import (
	"fmt"
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
	dMDNS, err := p2p.NewMDNS(h, args.Rendezvous)
	if err != nil {
		log.Fatal(err)
	}
	// For global-network
	// dDHT, err := newDHT(ctx, h, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	fmt.Printf("Peer: %s\n", h.ID())

	// Discover
	go dMDNS.Run( /*args.Network*/ )
	// go dDHT.run(args.Network)

	// Packet
	h.SetStreamHandler(sync.SyncProtocol, sync.SyncHandler())

	// synchornize
	sync.SyncWatcher(h)
}

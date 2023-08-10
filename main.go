package main

import (
	"fmt"
	"log"
)

func main() {
	// ctx := context.Background()

	// Arguments
	args, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}

	// P2P Host
	h, err := newP2P(args.Port)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// For local-network
	dMDNS, err := newMDNS(h, args.Rendezvous)
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
	go dMDNS.run( /*args.Network*/ )
	// go dDHT.run(args.Network)

	// Packet
	h.SetStreamHandler(syncProtocol, syncHandler())

	// synchornize
	syncWatcher(h)
}

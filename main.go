package main

import (
	"context"
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
	ctx := context.Background()

	// P2P Host
	node, err := p2p.NewNodeByLite(ctx, args.Port, args.Rendezvous)
	if err != nil {
		log.Fatal(err)
	}
	defer node.Close()

	// store, err := p2p.NewStore(ctx, node.Host, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer store.Close()

	// For local-network
	// dMDNS, err := p2p.NewMDNS(h, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// For global-network
	// dDHT, err := p2p.NewDHT(ctx, h, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Peer: %s\n", h.ID())

	// Discover
	// go dMDNS.Run( )
	// go dDHT.Run()
	// go node.Run()

	// // DHT
	// kadDHT := dDHT.DHT()

	// Packet
	node.Host.SetStreamHandler(sync.SyncProtocol, sync.SyncHandler(node))

	// synchornize
	sync.SyncWatcher(node, args.SyncDir)
}

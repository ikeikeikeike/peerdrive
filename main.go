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
	h, ddht, err := p2p.NewP2PByLite(ctx, args.Port)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// s, err := newStore(ctx, h, args.Rendezvous)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer s.Close()

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
	go p2p.RunDHT(ddht, args.Rendezvous)

	// // DHT
	// kadDHT := dDHT.DHT()

	// Packet
	h.SetStreamHandler(sync.SyncProtocol, sync.SyncHandler())

	// synchornize
	sync.SyncWatcher(h, args.SyncDir)
}

// type store struct {
// }
//
// func newStore(ctx context.Context, h host.Host, rendezvous string) (*ds.Datastore, error) {
// 	store, err := badger.NewDatastore("./datastore", nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	psub, err := pubsub.NewGossipSub(ctx, h)
// 	if err != nil {
// 		return nil, err
// 	}
// 	bcast, err := crdt.NewPubSubBroadcaster(ctx, psub, rendezvous)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	// more setups
// 	// DAGService
// 	// dags := ...
//
// 	opts := crdt.DefaultOptions()
// 	opts.RebroadcastInterval = 5 * time.Second
// 	opts.PutHook = func(k ds.Key, v []byte) {
// 		fmt.Printf("Added: [%s] -> %s\n", k, string(v))
// 	}
// 	opts.DeleteHook = func(k ds.Key) {
// 		fmt.Printf("Removed: [%s]\n", k)
// 	}
//
// 	crdt.New(store, ds.NewKey("crdt"), nil, bcast, opts)
//
// 	return nil, nil
// }
//
// // topic, err := psub.Join(netTopic)
// // if err != nil {
// // 	logger.Fatal(err)
// // }
// //
// // netSubs, err := topic.Subscribe()
// // if err != nil {
// // 	logger.Fatal(err)
// // }
// //
// // // Use a special pubsub topic to avoid disconnecting
// // // from globaldb peers.
// // go func() {
// // 	for {
// // 		msg, err := netSubs.Next(ctx)
// // 		if err != nil {
// // 			fmt.Println(err)
// // 			break
// // 		}
// // 		h.ConnManager().TagPeer(msg.ReceivedFrom, "keep", 100)
// // 	}
// // }()
// //
// // go func() {
// // 	for {
// // 		select {
// // 		case <-ctx.Done():
// // 			return
// // 		default:
// // 			topic.Publish(ctx, []byte("hi!"))
// // 			time.Sleep(20 * time.Second)
// // 		}
// // 	}
// // }()

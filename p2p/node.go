package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	crdt "github.com/ipfs/go-ds-crdt"

	"github.com/multiformats/go-multiaddr"
	"github.com/samber/lo"
)

var (
	defaultBootstrapPeers     = dht.DefaultBootstrapPeers
	defaultBootstrapPeersInfo []peer.AddrInfo
)

func init() {
	maddr, err := multiaddr.NewMultiaddr(
		"/ip4/104.131.131.82/udp/4001/quic/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	)
	if err != nil {
		panic(err)
	}
	defaultBootstrapPeers = append(defaultBootstrapPeers, maddr)

	infos, err := peer.AddrInfosFromP2pAddrs(defaultBootstrapPeers...)
	if err != nil {
		panic(err)
	}
	defaultBootstrapPeersInfo = infos
}

type PeerList []peer.ID

func (pl *PeerList) AppendUnique(ids ...peer.ID) bool {
	prevs := len(*pl)
	*pl = lo.Uniq(append(*pl, ids...))
	return prevs != len(*pl)
}

var Peers = PeerList{}

const (
	DSName = "snap"
)

type Node struct {
	Host       host.Host
	IPFS       *ipfslite.Peer
	DHT        *dual.DHT       // routing.Routing
	DS         *crdt.Datastore // datastore.Batching
	Rendezvous string
}

func (n *Node) Close() error {
	return multierror.Append(
		n.Host.Close(),
		n.DHT.Close(),
		n.DS.Close(),
	)
}

// Default Behavior: https://pkg.go.dev/github.com/libp2p/go-libp2p#New
func NewNodeByLite(ctx context.Context, port int, rendezvous string) (*Node, error) {
	pkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, err
	}

	maddrs := []multiaddr.Multiaddr{}
	for _, s := range []string{
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port),
		fmt.Sprintf("/ip6/::/tcp/%d", port),
		fmt.Sprintf("/ip6/::/udp/%d/quic", port),
	} {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return nil, err
		}
		maddrs = append(maddrs, ma)
	}

	p2pOpts := append([]libp2p.Option{}, ipfslite.Libp2pOptionsExtra...)
	p2pOpts = append(p2pOpts, []libp2p.Option{
		// libp2p.EnableAutoRelay(),
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.FallbackDefaults,
	}...)
	h, dht, err := ipfslite.SetupLibp2p(ctx, pkey, nil, maddrs, nil, p2pOpts...)
	if err != nil {
		return nil, err
	}

	badgerDS, err := badger.NewDatastore(fmt.Sprintf("./.%s", DSName), &badger.DefaultOptions)
	if err != nil {
		return nil, err
	}
	ipfs, err := ipfslite.New(ctx, badgerDS, nil, h, dht, nil)
	if err != nil {
		return nil, err
	}
	ipfs.Bootstrap(defaultBootstrapPeersInfo)

	psub, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, err
	}
	bcast, err := crdt.NewPubSubBroadcaster(ctx, psub, rendezvous)
	if err != nil {
		return nil, err
	}
	// more setups
	// DAGService
	// dags := ...
	crdtOpts := crdt.DefaultOptions()
	crdtOpts.RebroadcastInterval = 5 * time.Second
	crdtOpts.PutHook = func(k datastore.Key, v []byte) {
		fmt.Printf("Added: [%s] -> %s\n", k, string(v))
	}
	crdtOpts.DeleteHook = func(k datastore.Key) {
		fmt.Printf("Removed: [%s]\n", k)
	}
	crdtDS, err := crdt.New(badgerDS, datastore.NewKey(DSName), ipfs, bcast, crdtOpts)
	if err != nil {
		return nil, err
	}

	n := &Node{Host: h, DHT: dht, DS: crdtDS, IPFS: ipfs, Rendezvous: rendezvous}
	go n.run(psub)
	return n, nil
}

func (nd *Node) run(psub *pubsub.PubSub) {
	topic, err := psub.Join(fmt.Sprintf("%s-net", nd.Rendezvous))
	if err != nil {
		log.Fatalln(err)
	}
	netSubs, err := topic.Subscribe()
	if err != nil {
		log.Fatalln(err)
	}

	// Use a special pubsub topic to avoid disconnecting
	// from globaldb peers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			msg, err := netSubs.Next(ctx)
			if err != nil {
				fmt.Println(err)
				break
			}
			nd.Host.ConnManager().TagPeer(msg.ReceivedFrom, "keep", 100)
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				topic.Publish(ctx, []byte("hi!"))
				time.Sleep(20 * time.Second)
			}
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx := context.Background()

		rd := routing.NewRoutingDiscovery(nd.DHT)
		util.Advertise(ctx, rd, nd.Rendezvous)

		peerCh, err := rd.FindPeers(ctx, nd.Rendezvous)
		if err != nil {
			fmt.Printf("DHT FindPeers failed: %+v\n", err)
			continue
		}

		for p := range peerCh {
			if p.ID == nd.Host.ID() || len(p.Addrs) == 0 {
				continue
			}
			if err := nd.Host.Connect(ctx, p); err != nil {
				// fmt.Println("DHT Connection failed:", p.ID, ">>", err)
				continue
			}
			if Peers.AppendUnique(p.ID) {
				fmt.Printf("Connect peer by DHT: %s\n", p.ID)
			}
		}
	}
}

type discoveryMDNS struct {
	PeerCh chan peer.AddrInfo
	host   host.Host
}

func (n *discoveryMDNS) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerCh <- pi
}

func (n *discoveryMDNS) Run() {
	for {
		p := <-n.PeerCh
		if p.ID == n.host.ID() {
			continue
		}
		if err := n.host.Connect(context.Background(), p); err != nil {
			// fmt.Println("MDNS Connection failed:", p.ID, ">>", err)
			continue
		}
		if Peers.AppendUnique(p.ID) {
			fmt.Printf("Connect peer by MDNS: %s\n", p.ID)
		}
	}
}

func NewMDNS(h host.Host, rendezvous string) (*discoveryMDNS, error) {
	n := &discoveryMDNS{
		host:   h,
		PeerCh: make(chan peer.AddrInfo),
	}

	if err := mdns.NewMdnsService(h, rendezvous, n).Start(); err != nil {
		return nil, err
	}

	return n, nil
}

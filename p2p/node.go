package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"

	"github.com/multiformats/go-multiaddr"
	"github.com/samber/lo"
)

type PeerList []peer.ID

func (pl *PeerList) AppendUnique(ids ...peer.ID) bool {
	prevs := len(*pl)
	*pl = lo.Uniq(append(*pl, ids...))
	return prevs != len(*pl)
}

var Peers = PeerList{}

func NewP2PByLite(ctx context.Context, port int) (host.Host, *dual.DHT, error) {
	pkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
		}
		maddrs = append(maddrs, ma)
	}

	opts := append([]libp2p.Option{}, ipfslite.Libp2pOptionsExtra...)
	opts = append(opts, []libp2p.Option{
		// libp2p.EnableAutoRelay(),
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.FallbackDefaults,
	}...)

	h, dd, err := ipfslite.SetupLibp2p(ctx, pkey, nil, maddrs, nil, opts...)
	if err != nil {
		return nil, nil, err
	}

	maddr, err := multiaddr.NewMultiaddr(
		"/ip4/104.131.131.82/udp/4001/quic/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	)
	if err != nil {
		return nil, nil, err
	}
	boots := append(dht.DefaultBootstrapPeers, maddr)

	var wg sync.WaitGroup
	for _, pa := range boots {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(pa)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Printf("DHT Bootstrap Connection failed: %+v\n", err)
			}
		}()
	}
	wg.Wait()

	if err = dd.Bootstrap(ctx); err != nil {
		return nil, nil, err
	}

	return h, dd, nil
}

func NewP2P(ctx context.Context, port int) (host.Host, error) {
	pkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, err
	}

	addrs := libp2p.ListenAddrStrings(
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port),
		fmt.Sprintf("/ip6/::/tcp/%d", port),
		fmt.Sprintf("/ip6/::/udp/%d/quic", port),
	)

	// Default Behavior: https://pkg.go.dev/github.com/libp2p/go-libp2p#New
	return libp2p.New(
		addrs,
		libp2p.Identity(pkey),
		// libp2p.EnableAutoRelay(),
		libp2p.EnableNATService(),
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
		libp2p.DefaultMuxers,
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(websocket.New),
		libp2p.FallbackDefaults,
	)
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

type discoveryDHT struct {
	dht        *dht.IpfsDHT
	rendezvous string
}

func RunDHT(ddht *dual.DHT, rendezvous string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	host := ddht.LAN.Host()
	for range ticker.C {
		ctx := context.Background()

		rd := routing.NewRoutingDiscovery(ddht)
		util.Advertise(ctx, rd, rendezvous)

		peerCh, err := rd.FindPeers(ctx, rendezvous)
		if err != nil {
			fmt.Printf("DHT FindPeers failed: %+v\n", err)
			continue
		}

		for p := range peerCh {
			if p.ID == host.ID() || len(p.Addrs) == 0 {
				continue
			}
			if err := host.Connect(ctx, p); err != nil {
				// fmt.Println("DHT Connection failed:", p.ID, ">>", err)
				continue
			}
			if Peers.AppendUnique(p.ID) {
				fmt.Printf("Connect peer by DHT: %s\n", p.ID)
			}
		}
	}
}

func (n *discoveryDHT) Run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	host := n.dht.Host()
	for range ticker.C {
		ctx := context.Background()

		rd := routing.NewRoutingDiscovery(n.dht)
		util.Advertise(ctx, rd, n.rendezvous)

		peerCh, err := rd.FindPeers(ctx, n.rendezvous)
		if err != nil {
			fmt.Printf("DHT FindPeers failed: %+v\n", err)
			continue
		}

		for p := range peerCh {
			if p.ID == host.ID() || len(p.Addrs) == 0 {
				continue
			}
			if err := host.Connect(ctx, p); err != nil {
				// fmt.Println("DHT Connection failed:", p.ID, ">>", err)
				continue
			}
			if Peers.AppendUnique(p.ID) {
				fmt.Printf("Connect peer by DHT: %s\n", p.ID)
			}
		}
	}
}

func (n *discoveryDHT) DHT() *dht.IpfsDHT {
	return n.dht
}

func NewDHT(ctx context.Context, h host.Host, rendezvous string) (*discoveryDHT, error) {
	// dht.NamespacedValidator
	kadDHT, err := dht.New(ctx, h)
	if err != nil {
		return nil, err
	}
	maddr, err := multiaddr.NewMultiaddr(
		"/ip4/104.131.131.82/udp/4001/quic/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	)
	if err != nil {
		return nil, err
	}
	boots := append(dht.DefaultBootstrapPeers, maddr)

	var wg sync.WaitGroup
	for _, pa := range boots {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(pa)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Printf("DHT Bootstrap Connection failed: %+v\n", err)
			}
		}()
	}
	wg.Wait()

	if err = kadDHT.Bootstrap(ctx); err != nil {
		return nil, err
	}

	ddht := &discoveryDHT{
		dht:        kadDHT,
		rendezvous: rendezvous,
	}
	return ddht, nil
}

package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/multiformats/go-multiaddr"
	"github.com/samber/lo"
)

var (
	Peers = []peer.ID{}
)

func NewP2P(port int) (host.Host, error) {
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
		// fmt.Printf("Connect peer by MDNS: %s\n", p.ID)
		Peers = lo.Uniq(append(Peers, p.ID))
	}
}

func NewMDNS(h host.Host, rendezvous string) (*discoveryMDNS, error) {
	n := &discoveryMDNS{
		host:   h,
		PeerCh: make(chan peer.AddrInfo),
	}

	ser := mdns.NewMdnsService(h, rendezvous, n)
	if err := ser.Start(); err != nil {
		return nil, err
	}

	return n, nil
}

type discoveryDHT struct {
	dht        *dht.IpfsDHT
	rendezvous string
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
			fmt.Println("DHT FindPeers failed:", err)
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
			// fmt.Printf("Connect peer by DHT: %s\n", p.ID)
			Peers = lo.Uniq(append(Peers, p.ID))
		}
	}
}

func (n *discoveryDHT) DHT() *dht.IpfsDHT {
	return n.dht
}

func NewDHT(h host.Host, rendezvous string) (*discoveryDHT, error) {
	ctx := context.Background()

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
				fmt.Printf("DHT Bootstrap Connection failed: %v\n", err)
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

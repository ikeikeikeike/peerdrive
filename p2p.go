package main

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

const syncProtocol = "/peerdrive/1.0.0"

func newP2P(port int) (host.Host, error) {
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

func (n *discoveryMDNS) run( /*address string*/ ) {
	for {
		p := <-n.PeerCh
		if p.ID == n.host.ID() {
			continue
		}
		ctx := context.Background()

		if err := n.host.Connect(ctx, p); err != nil {
			// fmt.Println("MDNS Connection failed:", p.ID, ">>", err)
			continue
		}
		fmt.Printf("Connect peer: %s\n", p.ID)

		// Can be written
		// if err := discoveryWriter(ctx, n.host, address, p); err != nil {
		// 	// fmt.Println("MDNS writer failed:", p.ID, ">>", err)
		// 	continue
		// }
	}
}

func newMDNS(h host.Host, rendezvous string) (*discoveryMDNS, error) {
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

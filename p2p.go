package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/multiformats/go-multiaddr"
)

const syncProtocol = "/peerdrive/1.0.0"

func newP2P(port int) (host.Host, error) {
	pkey, err := privKey()
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

func privKey() (crypto.PrivKey, error) {
	// name := ".pkey"

	// Restore pkey
	// if _, err := os.Stat(name); !os.IsNotExist(err) {
	// 	dat, err := ioutil.ReadFile(name)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	//
	// 	return crypto.UnmarshalPrivateKey(dat)
	// }

	pkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, err
	}
	// Store Key
	// privBytes, err := crypto.MarshalPrivateKey(pkey)
	// if err != nil {
	// 	return nil, err
	// }
	// if err := ioutil.WriteFile(name, privBytes, 0644); err != nil {
	// 	return nil, err
	// }

	return pkey, nil
}

func node() {
	h, err := newP2P()
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	h.SetStreamHandler(syncProtocol, handleStream)
}

func sendFileChange(h host.Host) {
	targetAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmOtherPeerID")
	targetInfo, _ := peer.AddrInfoFromP2pAddr(targetAddr)

	stream, err := h.NewStream(context.Background(), targetInfo.ID, syncProtocol)
	if err != nil {
		log.Println("Error opening stream:", err)
		return
	}

	event := "" // event.Event().String()
	_, err = stream.Write([]byte(event))
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

package sync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io/ioutil"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/samber/lo"

	"github.com/ikeikeikeike/peerdrive/p2p"
	"github.com/ikeikeikeike/peerdrive/sync/event"
)

func notifyCopy(h host.Host, path, relPath string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("notify copy failed: %w", err)
	}

	ev := &event.Event{Op: event.Copy, Path: relPath, Data: data}
	return writeStreams(context.Background(), h, SyncProtocol, ev)
}

func notifyDelete(h host.Host, relPath string) error {
	ev := &event.Event{Op: event.Delete, Path: relPath}
	return writeStreams(context.Background(), h, SyncProtocol, ev)
}

func writeStreams(ctx context.Context, h host.Host, protocol protocol.ID, ev *event.Event) error {
	for _, peerID := range lo.Uniq(p2p.Peers) {
		if err := writeStream(ctx, h, protocol, peerID, ev); err != nil {
			return fmt.Errorf("%s write stream failed: %w", peerID, err)
		}
	}

	return nil
}

func writeStream(ctx context.Context, h host.Host, protocol protocol.ID, peerID peer.ID, ev *event.Event) error {
	stream, err := h.NewStream(ctx, peerID, protocol)
	if err != nil {
		return fmt.Errorf("%s stream open failed: %w", peerID, err)
	}
	defer stream.Close()

	b := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(b).Encode(ev); err != nil {
		return fmt.Errorf("%s error sending message encode: %w", peerID, err)
	}
	buf := b.Bytes()

	packetSize := make([]byte, 4)
	binary.BigEndian.PutUint32(packetSize, uint32(len(buf)))

	writer := bufio.NewWriter(stream)
	if _, err := writer.Write(packetSize); err != nil {
		return fmt.Errorf("%s error sending message length: %w", peerID, err)
	}
	// fmt.Printf("Write: %d\n", len(buf))
	if _, err := writer.Write(buf); err != nil {
		return fmt.Errorf("%s error sending message: %w", peerID, err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("%s error flushing writer: %w", peerID, err)
	}

	return nil
}

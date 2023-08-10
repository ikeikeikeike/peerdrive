package sync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/radovskyb/watcher"
	"github.com/samber/lo"

	"github.com/ikeikeikeike/peerdrive/sync/event"
)

const SyncProtocol = "/peerdrive/1.0.0"

// so tiny: need more handling
func logFatal(err error) {
	if err == nil {
		return
	}

	log.Fatalln(err)
}

func fileSize(name string) int64 {
	fi, err := os.Stat(name)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func fileMTime(name string) int64 {
	fi, err := os.Stat(name)
	if err != nil {
		return 0
	}
	mtime := fi.ModTime().UnixNano()
	return mtime
}

func isFileWritten(path string) (bool, int64) {
	mtime1 := fileMTime(path)
	time.Sleep(1 * time.Second)
	mtime2 := fileMTime(path)
	return mtime1 != mtime2, mtime2
}

func notifyCopy(h host.Host, path, peerPath string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("notify copy failed: %w", err)
	}

	ev := &event.Event{Op: event.Copy, Path: peerPath, Data: data}
	return writeStreams(context.Background(), h, SyncProtocol, ev)
}

func notifyDelete(h host.Host, peerPath string) error {
	ev := &event.Event{Op: event.Delete, Path: peerPath}
	return writeStreams(context.Background(), h, SyncProtocol, ev)
}

func writeStreams(ctx context.Context, h host.Host, protocol protocol.ID, ev *event.Event) error {
	dupIDs := lo.Map(h.Network().Conns(), func(conn network.Conn, _ int) peer.ID {
		return conn.RemotePeer()
	})
	for _, peerID := range lo.Uniq(dupIDs) {
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

func SyncHandler() func(stream network.Stream) {
	return func(stream network.Stream) {
		defer stream.Close()
		peerID := stream.Conn().RemotePeer()

		for {
			packetSize := make([]byte, 4)
			if _, err := io.ReadFull(stream, packetSize); err != nil {
				if err != io.EOF {
					log.Printf("%s error reading length from stream: %+v", peerID, err)
				}
				return
			}
			data := make([]byte, binary.BigEndian.Uint32(packetSize))
			if _, err := io.ReadFull(stream, data); err != nil {
				log.Printf("%s error reading message from stream: %+v", peerID, err)
				return
			}
			ev := &event.Event{}
			if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&ev); err != nil {
				log.Printf("%s error reading message from stream: %+v", peerID, err)
				return
			}

			var err error
			switch ev.Op {
			case event.Copy:
				err = ev.Copy()
				recvDispChanged(ev.Path)
			case event.Delete:
				err = ev.Delete()
				recvDispDeleted(ev.Path)
			}
			if err != nil {
				log.Printf("%s error operate message from stream: %+v", peerID, err)
				return
			}
		}
	}
}
func SyncWatcher(h host.Host) {
	w := watcher.New()
	watchCh := make(chan watcher.Event, 100)
	finCh := make(chan string, 100)

	baseDir := "./"
	currDir, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	syncs := []string{}
	go func() {
		for {
			select {
			case ev := <-w.Event:
				if strings.HasPrefix(ev.Name(), ".") {
					break
				}
				if ev.Op == watcher.Chmod {
					break
				}

				if !ev.IsDir() {
					notIn := true
					for _, v := range syncs {
						if v == ev.Path {
							notIn = false
						}
					}
					if notIn {
						syncs = append(syncs, ev.Path)
						watchCh <- ev
					}
				}
			case err := <-w.Error:
				log.Fatalln(err)
			case path := <-finCh:
				index := -1
				for i, v := range syncs {
					if v == path {
						index = i
					}
				}
				if index != -1 {
					syncs = append(syncs[:index], syncs[index+1:]...)
				}
			case <-w.Closed:
				return
			}
		}
	}()
	go func() {
		for {
			ev := <-watchCh

			time.Sleep(100 * time.Millisecond)
			size := fileSize(ev.Path)
			if size > 1024*20 {
				for {
					if isWritten, _ := isFileWritten(ev.Path); !isWritten {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			relPath := strings.ReplaceAll(ev.Path, currDir, "")
			relOldPath := strings.ReplaceAll(ev.OldPath, currDir, "")
			peerPath := path.Join(baseDir, relPath)

			switch ev.Op {
			case watcher.Move, watcher.Rename:
				logFatal(notifyCopy(h, ev.Path, peerPath))
				sendDispChanged(relPath)
				logFatal(notifyDelete(h, path.Join(baseDir, relOldPath)))
				sendDispDeleted(relPath)
			case watcher.Create, watcher.Write:
				logFatal(notifyCopy(h, ev.Path, peerPath))
				sendDispChanged(relPath)
			case watcher.Remove:
				logFatal(notifyDelete(h, peerPath))
				sendDispDeleted(relPath)
			}

			finCh <- ev.Path
		}
	}()

	if err := w.AddRecursive("./"); err != nil {
		log.Fatalln(err)
	}
	if err := w.Start(time.Millisecond * 500); err != nil {
		log.Fatalln(err)
	}
}

package sync

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/radovskyb/watcher"

	"github.com/ikeikeikeike/peerdrive/sync/event"
)

const SyncProtocol = "/peerdrive/1.0.0"

var (
	syncs = &SafeSlice[string]{}
	recvs = &SafeSlice[string]{}
)

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
			recvs.Append(ev.Path)
			var err error
			switch ev.Op {
			case event.Copy:
				err = ev.Copy()
				recvDispChanged(ev.Path)
			case event.Delete:
				err = ev.Delete()
				recvDispDeleted(ev.Path)
			}
			time.AfterFunc(time.Second, func() { recvs.Remove(ev.Path) })
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

	currDir, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

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
				if ev.IsDir() {
					break
				}
				relPath, _ := paths(currDir, ev)
				if syncs.Contains(relPath) {
					break
				}
				if recvs.Contains(relPath) {
					break
				}

				syncs.Append(relPath)
				watchCh <- ev
			case err := <-w.Error:
				log.Fatalln(err)
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

			relPath, relOldPath := paths(currDir, ev)
			switch ev.Op {
			case watcher.Move, watcher.Rename:
				logFatal(notifyCopy(h, ev.Path, relPath))
				sendDispChanged(relPath)
				logFatal(notifyDelete(h, relOldPath))
				sendDispDeleted(relPath)
			case watcher.Create, watcher.Write:
				logFatal(notifyCopy(h, ev.Path, relPath))
				sendDispChanged(relPath)
			case watcher.Remove:
				logFatal(notifyDelete(h, relPath))
				sendDispDeleted(relPath)
			}

			time.AfterFunc(time.Second, func() { syncs.Remove(relPath) })
		}
	}()

	if err := w.AddRecursive("./"); err != nil {
		log.Fatalln(err)
	}
	if err := w.Start(time.Millisecond * 300); err != nil {
		log.Fatalln(err)
	}
}

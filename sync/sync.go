package sync

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/k0kubun/pp"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/radovskyb/watcher"

	"github.com/ikeikeikeike/peerdrive/p2p"
	"github.com/ikeikeikeike/peerdrive/sync/event"
)

const SyncProtocol = "/peerdrive/1.0.0"

var (
	syncs = &SafeSlice[string]{}
	recvs = &SafeSlice[string]{}
)

func SyncHandler(nd *p2p.Node) func(stream network.Stream) {
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

			if runtime.GOOS != "darwin" {
				pp.Println(nd.DS.Get(context.Background(), datastore.NewKey("mydatakey")))
			}
		}
	}
}

func SyncWatcher(nd *p2p.Node, syncDir string) {
	var (
		h       = nd.Host
		w       = watcher.New()
		watchCh = make(chan watcher.Event, 100)
		err     error
	)

	if syncDir == "" {
		if syncDir, err = os.Getwd(); err != nil {
			log.Fatalln(err)
		}
	}
	syncDir, err = filepath.Abs(syncDir)
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
				relPath, _ := paths(syncDir, ev)
				if syncs.Contains(relPath) {
					break
				}
				if recvs.Contains(relPath) {
					break
				}

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
			untilWritten(ev.Path)

			relPath, oldPath := paths(syncDir, ev)
			syncs.Append(relPath)

			if runtime.GOOS == "darwin" {
				logFatal(nd.DS.Put(context.Background(), datastore.NewKey("mydatakey"), []byte("value 1")))
			}

			switch ev.Op {
			case watcher.Move, watcher.Rename:
				logFatal(notifyCopy(h, ev.Path, relPath))
				sendDispChanged(relPath)
				logFatal(notifyDelete(h, oldPath))
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
	if err := w.Ignore(".git", ".snapshot"); err != nil {
		log.Fatalln(err)
	}
	if err := w.Start(time.Millisecond * 300); err != nil {
		log.Fatalln(err)
	}
}

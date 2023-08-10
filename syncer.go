package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/k0kubun/pp"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/samber/lo"

	"github.com/radovskyb/watcher"
)

func syncWatcher(h host.Host) {
	w := watcher.New()
	watchCh := make(chan watcher.Event, 100)
	finCh := make(chan string, 100)

	remoteDir := "/tmp"
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	syncs := []string{}
	go func() {
		for {
			select {
			case event := <-w.Event:
				if strings.HasPrefix(event.Name(), ".") {
					break
				}
				if event.Op == watcher.Chmod {
					break
				}

				if !event.IsDir() {
					notIn := true
					for _, v := range syncs {
						if v == event.Path {
							notIn = false
						}
					}
					if notIn {
						syncs = append(syncs, event.Path)
						watchCh <- event
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

	// Syncronize the file
	go func() {
		for {
			event := <-watchCh

			relativePath := strings.ReplaceAll(event.Path, currentDir, "")
			relativeOldPath := strings.ReplaceAll(event.OldPath, currentDir, "")

			time.Sleep(100 * time.Millisecond)
			size := fileSize(event.Path)
			if size > 1024*20 {
				for {
					isWritten, _ := isFileWritten(event.Path)
					if !isWritten {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			localPath := event.Path
			remotePath := path.Join(remoteDir, relativePath)
			if (event.Op == watcher.Move) || (event.Op == watcher.Rename) {
				copyFile(h, localPath, remotePath) // I/O

				oldRemoteFilepath := path.Join(remoteDir, relativeOldPath)
				deleteFile(h, oldRemoteFilepath) // I/O
			} else if (event.Op == watcher.Create) || (event.Op == watcher.Write) {
				copyFile(h, localPath, remotePath) // I/O
			} else if event.Op == watcher.Remove {
				deleteFile(h, remotePath) // I/O
			}

			finCh <- event.Path
		}
	}()

	if err := w.AddRecursive("./"); err != nil {
		log.Fatalln(err)
	}
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}
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
	// fmt.Println(mtime1, mtime2)
	return mtime1 != mtime2, mtime2
}

func copyFile(h host.Host, localpath, remotepath string) {
	ctx := context.Background()
	remotedir := path.Dir(remotepath)

	// Create remote dir
	writeStreams(ctx, h, syncProtocol, fmt.Sprintf("MkdirAll: %s", remotedir))
	// Create destination file
	writeStreams(ctx, h, syncProtocol, fmt.Sprintf("Create: %s", remotepath))
	// Create source file
	writeStreams(ctx, h, syncProtocol, fmt.Sprintf("Open: %s", localpath))
	// Copy source file to destination file
	writeStreams(ctx, h, syncProtocol, fmt.Sprintf("io.Copy: dstFile, srcFile"))
}

func deleteFile(h host.Host, remotepath string) {
	ctx := context.Background()

	// Delete file
	writeStreams(ctx, h, syncProtocol, fmt.Sprintf("Remove: %s", remotepath))
}

func writeStreams(ctx context.Context, h host.Host, protocol protocol.ID, data string) error {
	dupIDs := lo.Map(h.Network().Conns(), func(conn network.Conn, _ int) peer.ID {
		return conn.RemotePeer()
	})
	for _, peerID := range lo.Uniq(dupIDs) {
		if err := writeStream(ctx, h, protocol, peerID, data); err != nil {
			return err
		}
	}

	return nil
}

func writeStream(ctx context.Context, h host.Host, protocol protocol.ID, peerID peer.ID, data string) error {
	stream, err := h.NewStream(ctx, peerID, protocol)
	if err != nil {
		return fmt.Errorf("Stream open failed %s: %w", peerID, err)
	}
	defer stream.Close()

	packetSize := make([]byte, 4)
	binary.BigEndian.PutUint32(packetSize, uint32(len(data)))

	writer := bufio.NewWriter(stream)
	if _, err := writer.Write(packetSize); err != nil {
		return fmt.Errorf("Error sending message length %s: %w", peerID, err)
	}
	if _, err := writer.WriteString(data); err != nil {
		return fmt.Errorf("Error sending message %s: %w", peerID, err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("Error flushing writer %s: %w", peerID, err)
	}

	return nil
}

func syncHandler() func(stream network.Stream) {
	return func(stream network.Stream) {
		defer stream.Close()
		peerID := stream.Conn().RemotePeer()

		for {
			packetSize := make([]byte, 4)

			if _, err := io.ReadFull(stream, packetSize); err != nil {
				if err != io.EOF {
					log.Printf("Error reading length from stream %s: %v", peerID, err)
				}
				return
			}

			data := make([]byte, binary.BigEndian.Uint32(packetSize))
			if _, err := io.ReadFull(stream, data); err != nil {
				log.Printf("Error reading message from stream %s: %v", peerID, err)
				return
			}

			pp.Printf("Read: %s\n", string(data))
		}
	}
}

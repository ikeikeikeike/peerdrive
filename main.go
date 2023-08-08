package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/k0kubun/pp"

	"github.com/radovskyb/watcher"
)

// const syncProtocol = "/peerdrive/1.0.0"

func main() {
	// h, err := libp2p.New()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer h.Close()
	// log.Println("Host created. We are:", h.ID())

	// h.SetStreamHandler(syncProtocol, handleStream)

	w := watcher.New()
	watchCh, finCh := make(chan watcher.Event, 100), make(chan string, 100)

	remoteDir := "/tmp"
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	color.New(color.FgLightWhite, color.Bold).Print("Watching Dir:")
	color.Green.Println(currentDir)
	color.Yellow.Printf("RemoteDir: %s\n", remoteDir)
	color.Cyan.Println("-------------------------")

	syncs := []string{}
	go func() {
		for {
			select {
			case event := <-w.Event:
				// Filter file
				if strings.HasPrefix(event.Name(), ".") {
					break
				}

				// Filter sync-go-config.json
				if strings.Compare(event.Name(), "sync-go-config.json") == 0 {
					break
				}

				// Filter event
				if event.Op == watcher.Chmod {
					break
				}

				if !event.IsDir() {
					not_in := true
					// Check if the file is syncronizing
					for _, v := range syncs {
						if v == event.Path {
							not_in = false
						}
					}
					if not_in {
						syncs = append(syncs, event.Path)
						watchCh <- event
					}
				}
			case err := <-w.Error:
				log.Fatalln(err)
			case path := <-finCh:
				index := -1
				// Get index
				for i, v := range syncs {
					if v == path {
						index = i
					}
				}
				// Delete from slice
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
					isWriting, _ := checkFileWriting(event.Path)
					if !isWriting {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			localFilepath := event.Path
			remoteFilepath := path.Join(remoteDir, relativePath)
			if (event.Op == watcher.Move) || (event.Op == watcher.Rename) {
				SftpCopyFile(localFilepath, remoteFilepath)

				fmt.Print("⫸ ")
				color.New(color.FgLightGreen, color.Bold).Print(" Changed ")
				color.Gray.Println(relativePath)

				oldRemoteFilepath := path.Join(remoteDir, relativeOldPath)
				SftpDeleteFile(oldRemoteFilepath)

				fmt.Print("⫸ ")
				color.New(color.FgLightRed, color.Bold).Print(" Deleted ")
				color.Gray.Println(relativeOldPath)

			} else if (event.Op == watcher.Create) || (event.Op == watcher.Write) {
				SftpCopyFile(localFilepath, remoteFilepath)

				fmt.Print("⫸ ")
				color.New(color.FgLightGreen, color.Bold).Print(" Changed ")
				color.Gray.Println(relativePath)

			} else if event.Op == watcher.Remove {
				SftpDeleteFile(remoteFilepath)

				fmt.Print("⫸ ")
				color.New(color.FgLightRed, color.Bold).Print(" Deleted ")
				color.Gray.Println(relativePath)
			}

			finCh <- event.Path
		}
	}()

	if err := w.AddRecursive("./"); err != nil {
		log.Fatalln(err)
	}

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}

	// c := make(chan notify.EventInfo, 1)
	//
	// if err := notify.Watch(".", c, notify.All); err != nil {
	// 	log.Fatal(err)
	// }
	// defer notify.Stop(c)
	//
	// for {
	// 	select {
	// 	case event := <-c:
	// 		log.Printf("Notified event: %v\n", event)
	// 		log.Println(event.Event().String())
	// 		log.Println(event.Path())
	// 		log.Println(event.Sys())
	// 		// sendFileChange(h, event)
	// 	}
	// }
}

// func sendFileChange(h host.Host, event notify.EventInfo) {
// 	targetAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmOtherPeerID")
// 	targetInfo, _ := peer.AddrInfoFromP2pAddr(targetAddr)
//
// 	stream, err := h.NewStream(context.Background(), targetInfo.ID, syncProtocol)
// 	if err != nil {
// 		log.Println("Error opening stream:", err)
// 		return
// 	}
//
// 	_, err = stream.Write([]byte(event.Event().String()))
// 	if err != nil {
// 		log.Println("Error sending file change event:", err)
// 		return
// 	}
// }

// func handleStream(s network.Stream) {
// 	buf := make([]byte, 1024)
// 	n, _ := s.Read(buf)
// 	log.Printf("Received file change event: %s\n", buf[:n])
// 	// log.Println(event.Event().String())
// }

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

func checkFileWriting(path string) (bool, int64) {
	mtime1 := fileMTime(path)
	time.Sleep(1 * time.Second)
	mtime2 := fileMTime(path)
	// fmt.Println(mtime1, mtime2)
	return mtime1 != mtime2, mtime2
}

func SftpCopyFile(localpath, remotepath string) {
	remotedir := path.Dir(remotepath)

	// Create remote dir
	pp.Printf("MkdirAll: %s\n", remotedir)

	// Create destination file
	pp.Printf("Create: %s\n", remotepath)

	// Create source file
	pp.Printf("Open: %s\n", localpath)

	// Copy source file to destination file
	pp.Printf("io.Copy: dstFile, srcFile\n")
}

func SftpDeleteFile(remotepath string) {
	// Delete file
	pp.Printf("Remove: %s", remotepath)
}

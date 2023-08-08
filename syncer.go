package main

import (
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/k0kubun/pp"
	"github.com/radovskyb/watcher"
)

func syncer() {
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
				copyFile(localPath, remotePath)
				deleteFile(path.Join(remoteDir, relativeOldPath))
			} else if (event.Op == watcher.Create) || (event.Op == watcher.Write) {
				copyFile(localPath, remotePath)
			} else if event.Op == watcher.Remove {
				deleteFile(remotePath)
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

func copyFile(localpath, remotepath string) {
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

func deleteFile(remotepath string) {
	// Delete file
	pp.Printf("Remove: %s", remotepath)
}

package sync

import (
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/samber/lo"
)

// so tiny: need more handling
func logFatal(err error) {
	if err == nil {
		return
	}

	log.Printf("logFatal: %+v\n", err) // log.Fatalln(err)
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

func untilWritten(path string) {
	time.Sleep(100 * time.Millisecond)
	size := fileSize(path)
	if size > 1024*20 {
		for {
			if isWritten, _ := isFileWritten(path); !isWritten {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func paths(baseDir string, ev watcher.Event) (string, string) {
	relPath := strings.ReplaceAll(ev.Path, baseDir, "")
	relOldPath := strings.ReplaceAll(ev.OldPath, baseDir, "")
	return path.Join("./", relPath), path.Join("./", relOldPath)
}

type SafeSlice[T comparable] struct {
	mu    sync.Mutex
	slice []T
}

func (s *SafeSlice[T]) Append(values ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.slice = append(s.slice, values...)
}

func (s *SafeSlice[T]) Remove(value T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	iex := -1
	for i, v := range s.slice {
		if v == value {
			iex = i
		}
	}
	if iex != -1 {
		s.slice = append(s.slice[:iex], s.slice[iex+1:]...)
	}
}

func (s *SafeSlice[T]) Copy() []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]T(nil), s.slice...)
}

func (s *SafeSlice[T]) Contains(value T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return lo.Contains(s.slice, value)
}

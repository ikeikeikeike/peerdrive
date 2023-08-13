package event

import (
	"fmt"
	"log"
	"os"
	"path"
)

type Op uint

const (
	Copy Op = iota
	Delete
)

var ops = map[Op]string{
	Copy:   "COPY",
	Delete: "DELETE",
}

func (e Op) String() string {
	if op, ok := ops[e]; ok {
		return op
	}
	return "???"
}

type Event struct {
	Op
	Path string
	Data []byte
	// PeerID peer.ID
}

func (ev *Event) Copy() error {
	// Create peer's dir
	dir := path.Dir(ev.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("%s error mkdirAll %s: %w", ev.String(), dir, err)
	}
	// Create peer's file
	f, err := os.Create(ev.Path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Write a data to peer's file
	if _, err := f.Write(ev.Data); err != nil {
		return fmt.Errorf("%s error write: %w", ev.String(), err)
	}

	return nil
}

func (ev *Event) Delete() error {
	if err := os.Remove(ev.Path); err != nil {
		return fmt.Errorf("%s error remove %s: %w", ev.String(), ev.Path, err)
	}
	return nil
}

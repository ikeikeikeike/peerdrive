package main

import (
	"flag"

	"github.com/pkg/errors"
)

type args struct {
	Rendezvous string
	Port       int
	SyncDir    string
}

func parseArgs() (*args, error) {
	a := &args{}

	flag.StringVar(&a.Rendezvous, "rv", "", "Rendezvous string like the only master key")
	flag.IntVar(&a.Port, "port", 6868, "vpn-mesh port")
	flag.StringVar(&a.SyncDir, "sdir", "./", "Synchornize directory")

	flag.Parse()

	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	for _, r := range []string{"rv"} {
		if !seen[r] {
			return nil, errors.Errorf("missing required -%s argument/flag\n", r)
		}
	}

	return a, nil
}

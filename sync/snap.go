// dirPath := "./"
// snaps, err := createSnap(dirPath)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// // Example snapshots
// local := snaps
// // local := []*FileSnap{
// // 	{Path: "/path/to/file1", Size: 1234, ModTime: time.Now(), IsDir: false},
// // 	{Path: "/path/to/file2", Size: 5678, ModTime: time.Now(), IsDir: false},
// // }
//
//	remote := []*FileSnap{
//		{Path: "/path/to/file2", Size: 5678, ModTime: time.Now().Add(-10 * time.Minute), IsDir: false},
//		{Path: "/path/to/file3", Size: 91011, ModTime: time.Now(), IsDir: false},
//	}
//
// diff := calcDiff(local, remote)
//
// pp.Printf("Added: %+v\n", diff.Add)
// pp.Printf("Deleted: %+v\n", diff.Delete)
// pp.Printf("Modified: %+v\n", diff.Modify)
package sync

import (
	"os"
	"path/filepath"
	"time"
)

type FileSnap struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

func createSnap(dirPath string) ([]*FileSnap, error) {
	snaps := []*FileSnap{}

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		snaps = append(snaps, &FileSnap{
			Path:    path,
			Name:    info.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return snaps, nil
}

type SnapDiff struct {
	Add    []*FileSnap
	Delete []*FileSnap
	Modify []*FileSnap
}

func calcDiff(local, remote []*FileSnap) *SnapDiff {
	lmap := make(map[string]*FileSnap)
	rmap := make(map[string]*FileSnap)

	for _, snap := range local {
		lmap[snap.Path] = snap
	}
	for _, snap := range remote {
		rmap[snap.Path] = snap
	}

	diff := &SnapDiff{}
	for path, lsnap := range lmap {
		rsnap, ok := rmap[path]

		if !ok {
			diff.Delete = append(diff.Delete, lsnap)
		} else if lsnap.Size != rsnap.Size {
			diff.Modify = append(diff.Modify, lsnap)
		} else if !lsnap.ModTime.Equal(rsnap.ModTime) {
			diff.Modify = append(diff.Modify, lsnap)
		}
	}

	for path, rsnap := range rmap {
		if _, ok := lmap[path]; !ok {
			diff.Add = append(diff.Add, rsnap)
		}
	}

	return diff
}

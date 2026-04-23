package server

import (
	"io/fs"

	appassets "github.com/inkyvoxel/go-spark"
)

var staticFS = mustSubFS(appassets.FS, "static")

func mustSubFS(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

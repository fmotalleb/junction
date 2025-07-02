package front

import (
	"embed"
	"io/fs"
)

//go:generate bash build.sh

//go:embed dist/*
var distFs embed.FS

func GetDist() (fs.FS, error) {
	return fs.Sub(distFs, "dist")
}

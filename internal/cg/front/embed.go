package front

import (
	"embed"
	"io/fs"
)

//go:generate npm i
//go:generate npm run build

//go:embed dist/*
var distFs embed.FS

func GetDist() (fs.FS, error) {
	return fs.Sub(distFs, "dist")
}

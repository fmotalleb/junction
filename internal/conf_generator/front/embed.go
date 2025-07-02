package front

import "embed"

//go:generate bash build.sh

//go:embed dist/*
var FrontFs embed.FS

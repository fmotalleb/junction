package git

//go:generate bash -c "git describe --tags --abbrev=0 > latest-tag.tmp"
//go:generate bash -c "git rev-parse --abbrev-ref HEAD > branch.tmp"
//go:generate bash -c "git rev-parse HEAD > commit-hash.tmp"
//go:generate bash -c "git log -1 --pretty=%B > commit-msg.tmp"

import (
	_ "embed"
)

//go:embed latest-tag.tmp
var LastTag string

//go:embed branch.tmp
var Branch string

//go:embed commit-hash.tmp
var CommitHash string

//go:embed commit-msg.tmp
var CommitMessage string

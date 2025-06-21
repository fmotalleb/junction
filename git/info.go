package git

import "time"

var (
	version = "v0.0.0-dev"
	commit  = "--"
	date    = "2025-06-21T15:24:40Z"
	branch  = "dev-branch"
)

func GetVersion() string {
	return version
}

func GetCommit() string {
	return commit
}

func GetDate() time.Time {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		panic(err)
	}
	return t
}

func GetBranch() string {
	return branch
}

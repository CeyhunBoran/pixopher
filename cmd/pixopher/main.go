// Command pixopher is a pixel art CLI for agents to draw with while humans
// watch live.
package main

import (
	"os"
	"runtime/debug"

	"github.com/CeyhunBoran/pixopher/internal/cli"
)

// Set at build time:
//
//	go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/pixopher
var (
	version = cli.DefaultVersion
	commit  = cli.DefaultCommit
	date    = cli.DefaultDate
)

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}.Resolve(debug.ReadBuildInfo)
	os.Exit(cli.Execute(info, os.Args[1:], os.Stdout, os.Stderr))
}

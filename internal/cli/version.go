package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Defaults for builds that were not given ldflags.
const (
	DefaultVersion = "dev"
	DefaultCommit  = "none"
	DefaultDate    = "unknown"
)

// BuildInfo describes the binary. Release builds set it via ldflags in main.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Resolve fills fields left at their defaults from the Go build information,
// so that `go install ...@version` and plain `go build` still report a
// version, commit and date.
func (b BuildInfo) Resolve(read func() (*debug.BuildInfo, bool)) BuildInfo {
	if b.Version == "" {
		b.Version = DefaultVersion
	}
	if b.Commit == "" {
		b.Commit = DefaultCommit
	}
	if b.Date == "" {
		b.Date = DefaultDate
	}
	bi, ok := read()
	if !ok {
		return b
	}
	if b.Version == DefaultVersion && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		b.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if b.Commit == DefaultCommit {
				b.Commit = s.Value
			}
		case "vcs.time":
			if b.Date == DefaultDate {
				b.Date = s.Value
			}
		}
	}
	return b
}

func (b BuildInfo) String() string {
	return fmt.Sprintf("pixopher %s (commit %s, built %s)", b.Version, b.Commit, b.Date)
}

func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), info.String())
			return err
		},
	}
}

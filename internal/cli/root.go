// Package cli builds the cobra command tree: commands generated from the op
// registry plus the non-op commands such as version and completion.
package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// NewRoot returns the root command. Build information is printed by the
// version command.
func NewRoot(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:   "pixopher",
		Short: "A pixel art CLI for agents to draw with while humans watch live",
		// Errors are printed once by Execute, and usage is only shown for
		// usage errors, which cobra prints itself.
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(newVersionCmd(info))
	return root
}

// Execute runs the command tree with args (without the program name) and
// returns the process exit code.
func Execute(info BuildInfo, args []string, stdout, stderr io.Writer) int {
	root := NewRoot(info)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
		// Until the error codes from docs/cli.md land, every failure is a
		// usage error.
		return 1
	}
	return 0
}

// Package cli builds the cobra command tree: commands generated from the op
// registry plus the non-op commands such as version, use and completion.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// config is what commands take from the outside world, so tests can
// replace it.
type config struct {
	ops     *ops.Registry
	backend engine.Backend
	getenv  func(string) string
	now     func() time.Time
}

func defaultConfig() config {
	return config{ops: ops.Default, backend: engine.Direct{}, getenv: os.Getenv, now: time.Now}
}

// state is shared by the commands of one run.
type state struct {
	cfg config
	// json is set once a command has read its flags; until then a failing
	// run scans the raw arguments to decide how to print the error.
	json *bool
}

// NewRoot returns the root command for the built-in ops. Build information
// is printed by the version command.
func NewRoot(info BuildInfo) *cobra.Command {
	return newRoot(info, &state{cfg: defaultConfig()})
}

func newRoot(info BuildInfo, st *state) *cobra.Command {
	root := &cobra.Command{
		Use:   "pixopher",
		Short: "A pixel art CLI for agents to draw with while humans watch live",
		// Execute prints errors once, in the format --json asks for.
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().Bool("json", false, "machine-readable output (PIXOPHER_JSON=1 turns it on everywhere)")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &errs.Error{Code: errs.Usage, Message: err.Error()}
	})
	root.AddCommand(newVersionCmd(info, st), newUseCmd(st))
	root.AddCommand(newHistoryCmds(st)...)
	root.AddCommand(newExportCmd(st))
	for _, op := range st.cfg.ops.All() {
		addOpCommand(root, op, st)
	}
	return root
}

// Execute runs the command tree with args (without the program name) and
// returns the process exit code (docs/cli.md#exit-codes).
func Execute(info BuildInfo, args []string, stdout, stderr io.Writer) int {
	return execute(info, defaultConfig(), args, stdout, stderr)
}

func execute(info BuildInfo, cfg config, args []string, stdout, stderr io.Writer) int {
	st := &state{cfg: cfg}
	root := newRoot(info, st)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd, err := root.ExecuteC()
	if err == nil {
		return errs.ExitOK
	}
	e, ok := errors.AsType[*errs.Error](err)
	if !ok {
		// Errors that are not ours come from cobra: unknown commands and
		// wrong argument counts.
		e = &errs.Error{Code: errs.Usage, Message: err.Error()}
	}
	if e.Code == errs.Usage && e.Hint == "" && cmd != nil {
		e.Hint = fmt.Sprintf("see `%s --help`", cmd.CommandPath())
	}
	if st.jsonOn(cmd, args) {
		_ = writeJSON(stdout, struct {
			OK    bool        `json:"ok"`
			Error *errs.Error `json:"error"`
		}{false, e})
	} else {
		_, _ = fmt.Fprintf(stderr, "error[%s]: %s\n", e.Code, e.Message)
		if e.Hint != "" {
			_, _ = fmt.Fprintf(stderr, "hint: %s\n", e.Hint)
		}
	}
	return errs.ExitCode(e.Code)
}

// jsonOn reports whether output is JSON: an explicit --json flag wins, then
// PIXOPHER_JSON. When cmd's flags were not (fully) parsed, as for an
// unknown command or a bad flag, args are scanned for --json instead.
func (st *state) jsonOn(cmd *cobra.Command, args []string) bool {
	if st.json != nil {
		return *st.json
	}
	if cmd != nil && cmd.Flags().Parsed() {
		if f := cmd.Flags().Lookup("json"); f != nil && f.Changed {
			return f.Value.String() == "true"
		}
	}
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			return true
		}
		if v, ok := strings.CutPrefix(a, "--json="); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
	}
	return st.envJSON()
}

// envJSON reports whether PIXOPHER_JSON is true (1, t, true, ... as
// strconv.ParseBool reads it).
func (st *state) envJSON() bool {
	b, _ := strconv.ParseBool(st.cfg.getenv("PIXOPHER_JSON"))
	return b
}

// writeJSON writes v as one line of JSON. HTML characters are not escaped,
// so hints such as `<file>` stay readable.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return &errs.Error{Code: errs.IO, Message: err.Error()}
	}
	return nil
}

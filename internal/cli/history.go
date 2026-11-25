package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/history"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// undo, redo and log work on the history log rather than through an op's
// Apply, so they are engine commands, not registry ops (D50). Their
// arguments still parse with ops.Split, like generated commands (D37).
var (
	countArg = ops.ArgSpec{Name: "n", Kind: ops.KindInt, Default: "1", Help: "how many transactions (entries with --step)"}
	stepFlag = ops.ArgSpec{Name: "step", Kind: ops.KindBool, Flag: true, Help: "count history entries (steps of a run) instead of transactions"}
	undoSpec = ops.Spec{
		Name:    "undo",
		Summary: "Undo your last transactions",
		Args: []ops.ArgSpec{
			countArg, stepFlag,
			{Name: "any", Kind: ops.KindBool, Flag: true, Help: "undo the latest transactions by anyone, not only yours"},
			{Name: "force", Kind: ops.KindBool, Flag: true, Help: "undo even when later changes touched the same pixels or header"},
		},
		Examples: []string{"undo", "undo 3", "undo --step", "undo --any --as reviewer"},
	}
	redoSpec = ops.Spec{
		Name:    "redo",
		Summary: "Redo what you last undid",
		Args: []ops.ArgSpec{
			countArg, stepFlag,
			{Name: "force", Kind: ops.KindBool, Flag: true, Help: "redo even when later changes touched the same pixels or header"},
		},
		Examples: []string{"redo", "redo 2", "redo --step"},
	}
	logSpec = ops.Spec{
		Name:     "log",
		Summary:  "Show the document's history, newest first",
		Args:     []ops.ArgSpec{{Name: "limit", Kind: ops.KindInt, Flag: true, Default: "20", Help: "how many entries to show; 0 shows all"}},
		Examples: []string{"log", "log --limit 5 --json"},
	}
)

func newHistoryCmds(st *state) []*cobra.Command {
	mutate := append([]ops.ArgSpec{fileFlag, jsonFlag}, mutateFlags...)
	return []*cobra.Command{
		historyCmd(st, undoSpec, mutate, func(cmd *cobra.Command, args, c ops.Args, path string) error {
			return st.runUndo(cmd, args, c, path, false)
		}),
		historyCmd(st, redoSpec, mutate, func(cmd *cobra.Command, args, c ops.Args, path string) error {
			return st.runUndo(cmd, args, c, path, true)
		}),
		historyCmd(st, logSpec, []ops.ArgSpec{fileFlag, jsonFlag}, st.runLog),
	}
}

// historyCmd builds a command whose arguments parse like a generated op
// command's, on a document resolved the same way.
func historyCmd(st *state, spec ops.Spec, common []ops.ArgSpec,
	run func(cmd *cobra.Command, args, c ops.Args, path string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:                useLine(spec.Name, spec),
		Short:              spec.Summary,
		Long:               longHelp(spec),
		Example:            examples(spec),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, tokens []string) error {
			if wantsHelp(tokens) {
				return cmd.Help()
			}
			pos, flags, c, err := st.split(spec.Args, common, tokens)
			if err != nil {
				return err
			}
			args, path, err := st.resolve(spec.Args, c, pos, flags)
			if err != nil {
				return err
			}
			return run(cmd, args, c, path)
		},
	}
	for _, a := range spec.Args {
		if a.Flag {
			addHelpFlag(cmd.Flags(), a)
		}
	}
	for _, a := range common {
		if a.Flag && a.Name != "json" {
			addHelpFlag(cmd.Flags(), a)
		}
	}
	return cmd
}

func (st *state) runUndo(cmd *cobra.Command, args, c ops.Args, path string, redo bool) error {
	n := args.Int("n")
	if n < 1 {
		return errs.New(errs.Usage, "n must be at least 1, not %d", n).WithArg("n")
	}
	req := history.Request{
		Redo: redo, N: n, Step: args.Bool("step"), Any: args.Bool("any"), Force: args.Bool("force"),
	}
	res, err := st.cfg.backend.Undo(path, req, st.options(c))
	return st.report(cmd, res, err)
}

// logEntry is one entry of `log --json`: a history entry without its undo
// deltas, plus whether it is currently undone.
type logEntry struct {
	Rev    int      `json:"rev"`
	Tx     string   `json:"tx"`
	TS     string   `json:"ts"`
	Author string   `json:"author"`
	Kind   string   `json:"kind,omitempty"`
	Target int      `json:"target,omitempty"`
	Msg    string   `json:"msg,omitempty"`
	Ops    []string `json:"ops"`
	BBox   []int    `json:"bbox,omitempty"`
	Undone bool     `json:"undone,omitempty"`
}

func (st *state) runLog(cmd *cobra.Command, args, _ ops.Args, path string) error {
	limit := args.Int("limit")
	if limit < 0 {
		return errs.New(errs.Usage, "limit must be 0 or more, not %d", limit).WithArg("limit")
	}
	d, chain, err := st.cfg.backend.History(path)
	if err != nil {
		return err
	}
	entries := []logEntry{}
	for _, e := range slices.Backward(chain.Entries) {
		if limit > 0 && len(entries) == limit {
			break
		}
		lines := e.Ops
		if lines == nil {
			lines = []string{}
		}
		entries = append(entries, logEntry{
			Rev: e.Rev, Tx: e.Tx, TS: e.TS, Author: e.Author, Kind: e.Kind, Target: e.Target,
			Msg: e.Msg, Ops: lines, BBox: e.BBox, Undone: e.Kind == "" && chain.Undone(e.Rev),
		})
	}
	warnings := chain.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	if *st.json {
		return writeJSON(cmd.OutOrStdout(), struct {
			OK       bool       `json:"ok"`
			Rev      int        `json:"rev"`
			Entries  []logEntry `json:"entries"`
			Warnings []string   `json:"warnings"`
		}{true, d.Rev, entries, warnings})
	}
	return printLog(cmd.OutOrStdout(), cmd.ErrOrStderr(), entries, warnings)
}

// printLog writes one line per entry, newest first:
// `rev 3  t_0003  cli  2026-11-25T09:00:00Z  px 6,8 ink  -m eyes`. An undo
// or redo shows `undo rev 2`, and an entry that is currently undone ends in
// `  (undone)`.
func printLog(out, errOut io.Writer, entries []logEntry, warnings []string) error {
	for _, w := range warnings {
		_, _ = fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	var b strings.Builder
	if len(entries) == 0 {
		b.WriteString("no history\n")
	}
	for _, e := range entries {
		what := strings.Join(e.Ops, "; ")
		if e.Kind == pxofile.KindUndo || e.Kind == pxofile.KindRedo {
			what = fmt.Sprintf("%s rev %d", e.Kind, e.Target)
		}
		fmt.Fprintf(&b, "rev %d  %s  %s  %s  %s", e.Rev, e.Tx, e.Author, e.TS, what)
		if e.Msg != "" {
			b.WriteString("  -m " + ops.Quote(e.Msg))
		}
		if e.Undone {
			b.WriteString("  (undone)")
		}
		b.WriteString("\n")
	}
	if _, err := io.WriteString(out, b.String()); err != nil {
		return &errs.Error{Code: errs.IO, Message: err.Error()}
	}
	return nil
}

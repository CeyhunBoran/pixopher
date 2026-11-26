package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/script"
)

// maxPace is the longest --pace delay. Pacing is cosmetic, so a longer one
// is a mistake (D63).
const maxPace = 10 * time.Second

// run applies a batch script through the engine, so like undo it is not a
// registry op. Its arguments parse with ops.Split like a generated
// command's (D37).
var runSpec = ops.Spec{
	Name:    "run",
	Summary: "Apply a batch script as one transaction",
	Args: []ops.ArgSpec{
		{Name: "script", Kind: ops.KindString, Help: "script file, or - for stdin (default: stdin)"},
		{Name: "pace", Kind: ops.KindString, Flag: true, Help: "delay between ops for live viewers, such as 40ms (from serve, M7)"},
		{Name: "continue-on-error", Kind: ops.KindBool, Flag: true, Help: "skip lines that fail and report them as warnings"},
	},
	Examples: []string{"run draw.pxs -m 'block in'", "run - < draw.pxs", "run draw.pxs --continue-on-error --json"},
}

func newRunCmd(st *state) *cobra.Command {
	mutate := append([]ops.ArgSpec{fileFlag, jsonFlag}, mutateFlags...)
	return historyCmd(st, runSpec, mutate, st.runScript)
}

func (st *state) runScript(cmd *cobra.Command, args, c ops.Args, path string) error {
	opt := st.options(c)
	opt.ContinueOnError = args.Bool("continue-on-error")
	if args.Has("pace") {
		pace, err := time.ParseDuration(args.String("pace"))
		if err != nil || pace < 0 || pace > maxPace {
			return errs.New(errs.Usage, "bad pace %q", args.String("pace")).
				WithHint("pace is a duration from 0 to %s, such as 40ms", maxPace).WithArg("pace")
		}
		// Direct mode has no live viewer to pace for, so nothing sets
		// opt.OnOp and the script runs at full speed (D63).
		opt.Pace = pace
	}
	src, err := readScript(cmd, args.String("script"))
	if err != nil {
		return err
	}
	parsed, err := script.Parse(src, st.cfg.ops, opt.ContinueOnError)
	if err != nil {
		return err
	}
	steps := make([]engine.Step, len(parsed))
	for i, s := range parsed {
		if s.Msg == "" {
			// -m captions the steps that have no `--- message` (D62).
			s.Msg = c.String("message")
		}
		steps[i] = engine.Step{Msg: s.Msg, Calls: s.Calls}
	}
	res, err := st.cfg.backend.Apply(path, steps, opt)
	return st.report(cmd, res, err)
}

// readScript reads the script at name, or stdin for "" and "-".
func readScript(cmd *cobra.Command, name string) (string, error) {
	var (
		data []byte
		err  error
	)
	if name == "" || name == "-" {
		data, err = io.ReadAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			return "", errs.New(errs.IO, "script %s does not exist", name).WithArg("script")
		}
	}
	if err != nil {
		return "", (&errs.Error{Code: errs.IO, Message: err.Error()}).WithArg("script")
	}
	return string(data), nil
}

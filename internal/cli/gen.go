package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// Flags every generated command takes besides its op's own
// (docs/cli.md#common-flags-all-mutating-commands). --json is also a
// persistent root flag, so help lists it under the global flags.
var (
	fileFlag = ops.ArgSpec{
		Name: "file", Short: "f", Kind: ops.KindString, Flag: true,
		Help: "document to act on (default: $PIXOPHER_FILE, the file set by use, or the only *.pxo here)",
	}
	jsonFlag    = ops.ArgSpec{Name: "json", Kind: ops.KindBool, Flag: true}
	mutateFlags = []ops.ArgSpec{
		{Name: "message", Short: "m", Kind: ops.KindString, Flag: true, Help: "caption for this transaction"},
		{Name: "as", Kind: ops.KindString, Flag: true, Help: "author label (default $PIXOPHER_AUTHOR or cli)"},
		{Name: "expect-rev", Kind: ops.KindInt, Flag: true, Help: "fail with rev_conflict unless the document is at this rev"},
		dryRunFlag,
	}
	dryRunFlag = ops.ArgSpec{Name: "dry-run", Kind: ops.KindBool, Flag: true, Help: "report the change without writing anything"}
	// Creating ops (D39) take no -f: the file is their own positional.
	// They write no history entry, so -m, --as and --expect-rev don't apply.
	createFlags = []ops.ArgSpec{
		dryRunFlag,
		{Name: "force", Kind: ops.KindBool, Flag: true, Help: "replace an existing document and remove its history"},
	}
)

// addOpCommand adds the command generated from op's spec. A name with
// spaces, such as "palette add", becomes a subcommand of a group command.
// It panics on specs it can't turn into a command, which any test that
// builds the tree catches.
func addOpCommand(root *cobra.Command, op ops.Op, st *state) {
	spec := op.Spec()
	if !spec.Mutates {
		// Observation ops need an output channel in ops.Op; it arrives with
		// the first one (info, show).
		panic(fmt.Sprintf("cli: %s: observation ops are not supported yet", spec.Name))
	}
	common := append([]ops.ArgSpec{fileFlag, jsonFlag}, mutateFlags...)
	helpFlags := append([]ops.ArgSpec{fileFlag}, mutateFlags...)
	if spec.Creates {
		if _, ok := op.(ops.Creator); !ok {
			panic(fmt.Sprintf("cli: %s: a creating op must implement ops.Creator", spec.Name))
		}
		common = append([]ops.ArgSpec{jsonFlag}, createFlags...)
		helpFlags = createFlags
	}
	reserved := map[string]bool{"help": true, "h": true}
	for _, c := range common {
		reserved[c.Name] = true
		if c.Short != "" {
			reserved[c.Short] = true
		}
	}
	for _, a := range spec.Args {
		if a.Flag && (reserved[a.Name] || a.Short != "" && reserved[a.Short]) {
			panic(fmt.Sprintf("cli: %s: flag %q clashes with a common flag", spec.Name, a.Name))
		}
	}

	words := strings.Fields(spec.Name)
	parent := root
	for _, w := range words[:len(words)-1] {
		parent = groupCommand(parent, w)
	}
	cmd := &cobra.Command{
		Use:     useLine(words[len(words)-1], spec),
		Short:   spec.Summary,
		Long:    longHelp(spec),
		Example: examples(spec),
		Args:    cobra.ArbitraryArgs,
		// ops.Split parses the flags, so batch scripts and the CLI read
		// arguments the same way and negative coordinates (-2,-2) are not
		// mistaken for flags. The flag set below only feeds help.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, tokens []string) error {
			return st.runOp(cmd, op, common, tokens)
		},
	}
	for _, a := range append(slices.Clone(spec.Args), helpFlags...) {
		if a.Flag {
			addHelpFlag(cmd.Flags(), a)
		}
	}
	parent.AddCommand(cmd)
}

// groupCommand returns parent's subcommand called name, creating a command
// that only groups others when there is none.
func groupCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	c := &cobra.Command{Use: name, Short: name + " commands", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		return c.Help()
	}}
	parent.AddCommand(c)
	return c
}

func (st *state) runOp(cmd *cobra.Command, op ops.Op, common []ops.ArgSpec, tokens []string) error {
	for _, t := range tokens {
		if t == "--" {
			break
		}
		if t == "-h" || t == "--help" {
			return cmd.Help()
		}
	}
	spec := op.Spec()
	pos, flags, err := ops.Split(append(slices.Clone(common), spec.Args...), tokens)
	if err != nil {
		return err
	}
	c, err := ops.Build(common, nil, flags)
	if err != nil {
		return err
	}
	jsonOut := st.envJSON()
	if c.Has("json") {
		jsonOut = c.Bool("json")
	}
	st.json = &jsonOut

	if spec.Creates {
		return st.runCreate(cmd, op, c, pos, flags)
	}
	file := c.String("file")
	if len(pos) > 0 && strings.HasSuffix(pos[0], engine.Ext) {
		if file != "" {
			return errs.New(errs.Usage, "the document is given twice: -f %s and %s", file, pos[0]).WithArg("file")
		}
		file, pos = pos[0], pos[1:]
	}
	args, err := ops.Build(spec.Args, pos, flags)
	if err != nil {
		return err
	}
	path, err := engine.ResolveFile(file, st.cfg.getenv("PIXOPHER_FILE"))
	if err != nil {
		return err
	}
	opt := engine.Options{Author: c.String("as"), DryRun: c.Bool("dry-run"), Now: st.cfg.now}
	if opt.Author == "" {
		opt.Author = st.cfg.getenv("PIXOPHER_AUTHOR")
	}
	if c.Has("expect-rev") {
		n := c.Int("expect-rev")
		opt.ExpectRev = &n
	}
	call := ops.Call{Op: op, Args: args, Line: ops.Format(spec, args)}
	res, err := st.cfg.backend.Apply(path, []engine.Step{{Msg: c.String("message"), Calls: []ops.Call{call}}}, opt)
	return st.report(cmd, res, err)
}

// runCreate runs a creating op such as new (D39): the document is the op's
// own file positional, so there is no file resolution.
func (st *state) runCreate(cmd *cobra.Command, op ops.Op, c ops.Args, pos []string, flags map[string]string) error {
	spec := op.Spec()
	args, err := ops.Build(spec.Args, pos, flags)
	if err != nil {
		return err
	}
	opt := engine.Options{DryRun: c.Bool("dry-run"), Force: c.Bool("force"), Now: st.cfg.now}
	call := ops.Call{Op: op, Args: args, Line: ops.Format(spec, args)}
	res, err := st.cfg.backend.Create(args.String("file"), call, opt)
	return st.report(cmd, res, err)
}

// report prints a transaction result, or returns its error.
func (st *state) report(cmd *cobra.Command, res engine.Result, err error) error {
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); !ok {
			// Every engine error is structured; anything else comes from
			// the file system.
			err = &errs.Error{Code: errs.IO, Message: err.Error()}
		}
		return err
	}
	if *st.json {
		return writeJSON(cmd.OutOrStdout(), res)
	}
	return printResult(cmd.OutOrStdout(), cmd.ErrOrStderr(), res)
}

// printResult writes the human form of a transaction result:
// `ok rev 13  +24px  bbox 6,1 4,3`, with `  (dry run)` appended for a dry
// run and warnings on stderr.
func printResult(out, errOut io.Writer, res engine.Result) error {
	line := fmt.Sprintf("ok rev %d  +%dpx", res.Rev, res.Changed)
	if len(res.BBox) == 4 {
		line += fmt.Sprintf("  bbox %d,%d %d,%d", res.BBox[0], res.BBox[1], res.BBox[2], res.BBox[3])
	}
	if res.DryRun {
		line += "  (dry run)"
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	if _, err := fmt.Fprintln(out, line); err != nil {
		return &errs.Error{Code: errs.IO, Message: err.Error()}
	}
	return nil
}

// useLine is the usage line: `rect <at> <size> <color> [--fill]`-style,
// with `[name]` for optional and `...` for repeated positionals. Cobra
// appends [flags].
func useLine(name string, spec ops.Spec) string {
	parts := []string{name}
	var group []string
	required := false
	for _, a := range spec.Args {
		switch {
		case a.Flag:
		case a.Repeat:
			group = append(group, "<"+a.Name+">")
			required = a.Required
		case a.Required:
			parts = append(parts, "<"+a.Name+">")
		default:
			parts = append(parts, "["+a.Name+"]")
		}
	}
	if len(group) > 0 {
		// A repeat group (D40): `<at> <color> [<at> <color> ...]`.
		one := strings.Join(group, " ")
		if required {
			parts = append(parts, one)
		}
		parts = append(parts, "["+one+" ...]")
	}
	return strings.Join(parts, " ")
}

var kindHelp = map[ops.ArgKind]string{
	ops.KindString: "text",
	ops.KindInt:    "integer",
	ops.KindPoint:  "point x,y",
	ops.KindSize:   "size w,h or wxh",
	ops.KindRect:   "rect x,y,w,h or x,y w,h",
	ops.KindColor:  "color: key, name, ramp step, #hex or .",
	ops.KindEnum:   "one of",
	ops.KindGrid:   "letter grid",
	ops.KindArea:   "rect x,y,w,h or x,y w,h, or one pixel x,y",
}

// longHelp is the summary followed by one line per positional argument.
func longHelp(spec ops.Spec) string {
	var b strings.Builder
	b.WriteString(spec.Summary)
	width := 0
	for _, a := range spec.Args {
		if !a.Flag {
			width = max(width, len(a.Name))
		}
	}
	if width == 0 {
		return b.String()
	}
	b.WriteString("\n\nArguments:\n")
	for _, a := range spec.Args {
		if a.Flag {
			continue
		}
		kind := kindHelp[a.Kind]
		if a.Kind == ops.KindEnum {
			kind += " " + strings.Join(a.Enum, ", ")
		}
		line := fmt.Sprintf("  %-*s  %s", width, a.Name, kind)
		if a.Help != "" {
			line += "; " + a.Help
		}
		if a.Default != "" {
			line += fmt.Sprintf(" (default %s)", a.Default)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func examples(spec ops.Spec) string {
	lines := make([]string, len(spec.Examples))
	for i, e := range spec.Examples {
		lines[i] = "  pxo " + e
	}
	return strings.Join(lines, "\n")
}

// helpValue shows a flag's kind and default in help. Generated commands
// parse flags with ops.Split, so Set is never used for parsing.
type helpValue struct {
	kind ops.ArgKind
	text string
}

func (v *helpValue) String() string     { return v.text }
func (v *helpValue) Set(s string) error { v.text = s; return nil }
func (v *helpValue) Type() string       { return v.kind.String() }

func addHelpFlag(fs *pflag.FlagSet, a ops.ArgSpec) {
	help := a.Help
	if a.Kind == ops.KindEnum {
		help = strings.TrimSpace(help + " (one of: " + strings.Join(a.Enum, ", ") + ")")
	}
	if a.Kind == ops.KindBool {
		fs.BoolP(a.Name, a.Short, a.Default == "true", help)
		return
	}
	fs.VarP(&helpValue{kind: a.Kind, text: a.Default}, a.Name, a.Short, help)
}

// Package script reads batch scripts, the input of `pxo run`
// (docs/agent-workflow.md#batch-scripts), into steps of op calls.
package script

import (
	"slices"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// Step is the calls between two `---` lines. Msg is the text after its
// `---`, or "" for the lines before the first `---` and a bare `---`.
type Step struct {
	Msg   string
	Calls []ops.Call
}

// Parse reads src into steps of calls to reg's mutating ops. Each call
// carries its 1-based line (ops.Call.LineNo). Steps without calls are
// left out.
//
// A line that can't be read (an unknown op, a malformed argument, an
// unterminated quote) fails the parse with a script_error naming it. With
// keepGoing, it becomes a call that fails with that error when applied
// instead, so `run --continue-on-error` skips it in line order with the
// others. Parse never reads a document: colors, layers and bounds are
// checked when the calls apply.
func Parse(src string, reg *ops.Registry, keepGoing bool) ([]Step, error) {
	var (
		steps []Step
		cur   Step
	)
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		n := i + 1
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if msg, ok := stepLine(line); ok {
			if len(cur.Calls) > 0 {
				steps = append(steps, cur)
			}
			cur = Step{Msg: msg}
			continue
		}
		var call ops.Call
		tokens, block, err := lineTokens(line, lines, &i)
		if err == nil {
			call, err = parseCall(tokens, block, reg)
		}
		if err != nil {
			if !keepGoing {
				return nil, errs.AtLine(n, err)
			}
			call = ops.Call{Op: failOp{err}, Line: line}
		}
		if call.Op == nil {
			continue // blank or comment
		}
		call.LineNo = n
		cur.Calls = append(cur.Calls, call)
	}
	if len(cur.Calls) > 0 {
		steps = append(steps, cur)
	}
	return steps, nil
}

// stepLine reports whether line starts a step (`---` alone or followed by
// a space and a message) and returns the message, trimmed and taken as is.
func stepLine(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "---")
	if !ok || rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// lineTokens splits line, the one at index *i of lines, into tokens. A
// line that ends in a bare `<<` starts a grid block: the lines after it, up
// to one that is `>>`, are the grid's rows, and they take the place of the
// `<<` as one token (ops.GridText). *i then moves past the `>>`, so the
// block is consumed even when the line fails; block reports that it was
// there. A blank or comment line gives no tokens.
func lineTokens(line string, lines []string, i *int) (tokens []string, block bool, err error) {
	if strings.HasPrefix(line, "#") {
		return nil, false, nil
	}
	tokens, bare, err := fields(line)
	if err != nil || len(tokens) == 0 {
		return nil, false, err
	}
	last := len(tokens) - 1
	t := tokens[last]
	if !bare[last] || !strings.HasPrefix(t, "<<") {
		return tokens, false, nil
	}
	if t != "<<" {
		return nil, false, errs.New(errs.Usage, "a grid block starts with a bare <<, not %q", t).
			WithHint("end the line with << and close the block with >> on its own line; quote a one-row grid that starts with <<")
	}
	var rows []string
	for *i++; *i < len(lines); *i++ {
		row := strings.TrimSpace(strings.TrimSuffix(lines[*i], "\r"))
		if row == ">>" {
			grid, err := ops.GridText(strings.Join(rows, "\n"))
			if err != nil {
				return nil, true, err
			}
			return append(tokens[:last], grid), true, nil
		}
		rows = append(rows, row)
	}
	return nil, true, errs.New(errs.Usage, "the grid block is not closed").
		WithHint("end it with >> on its own line; the block ran to the end of the script")
}

// parseCall turns a line's tokens into a call; block says the last token
// came from a grid block. No tokens give a call without an Op.
func parseCall(tokens []string, block bool, reg *ops.Registry) (ops.Call, error) {
	if len(tokens) == 0 {
		return ops.Call{}, nil
	}
	if tokens[0] == "@set" {
		return ops.ParseSet(tokens[1:])
	}
	if strings.HasPrefix(tokens[0], "@") {
		return ops.Call{}, errs.New(errs.Usage, "unknown directive %q", tokens[0]).
			WithHint("the only directive is @set layer=<name|index> frame=<index>")
	}
	op, rest, err := lookup(reg, tokens)
	if err != nil {
		return ops.Call{}, err
	}
	spec := op.Spec()
	if block && !slices.ContainsFunc(spec.Args, func(a ops.ArgSpec) bool { return !a.Flag && a.Kind == ops.KindGrid }) {
		return ops.Call{}, errs.New(errs.Usage, "%s takes no grid block", spec.Name).
			WithHint("only ops with a grid argument, such as grid, take a << block")
	}
	args, err := ops.Parse(spec.Args, rest)
	if err != nil {
		return ops.Call{}, err
	}
	return ops.Call{Op: op, Args: args, Line: ops.Format(spec, args)}, nil
}

// lookup finds the op a line calls: a two-word name such as `palette add`
// first, then a one-word one. Only ops that change an existing document
// can run in a script.
func lookup(reg *ops.Registry, tokens []string) (ops.Mutator, []string, error) {
	name, rest := tokens[0], tokens[1:]
	op, ok := reg.Lookup(name)
	if len(tokens) > 1 {
		if two, ok2 := reg.Lookup(name + " " + tokens[1]); ok2 {
			op, ok, name, rest = two, true, name+" "+tokens[1], tokens[2:]
		} else if !ok && isGroup(reg, name) {
			name = name + " " + tokens[1]
		}
	}
	if !ok {
		var names []string
		for _, o := range reg.All() {
			if s := o.Spec(); s.Mutates && !s.Creates {
				names = append(names, s.Name)
			}
		}
		e := errs.New(errs.Usage, "unknown op %q", name)
		if best := color.Closest(name, names); best != "" {
			e = e.WithHint("did you mean %q?", best)
		}
		return nil, nil, e
	}
	spec := op.Spec()
	switch {
	case spec.Creates:
		return nil, nil, errs.New(errs.Usage, "%s makes a new document, so it can't run in a script", name).
			WithHint("run `pxo %s` before the script", name)
	case !spec.Mutates:
		return nil, nil, errs.New(errs.Usage, "%s doesn't change the document, so it can't run in a script", name).
			WithHint("run `pxo %s` after the script", name)
	}
	return op.(ops.Mutator), rest, nil
}

// isGroup reports whether name is the first word of a two-word op name.
func isGroup(reg *ops.Registry, name string) bool {
	return slices.ContainsFunc(reg.All(), func(o ops.Op) bool {
		return strings.HasPrefix(o.Spec().Name, name+" ")
	})
}

// failOp stands for a line that could not be read, so --continue-on-error
// skips it when the calls apply.
type failOp struct{ err error }

func (failOp) Spec() ops.Spec {
	return ops.Spec{Name: "(unreadable line)", Group: "script", Summary: "a script line that could not be read", Mutates: true}
}

func (f failOp) Apply(*ops.Ctx, ops.Args) error { return f.err }

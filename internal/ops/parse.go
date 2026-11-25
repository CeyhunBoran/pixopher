package ops

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// Parse parses command-line style tokens (positionals and flags) into Args
// for the arguments in specs. It is Split followed by Build.
func Parse(specs []ArgSpec, tokens []string) (Args, error) {
	pos, flags, err := Split(specs, tokens)
	if err != nil {
		return nil, err
	}
	return Build(specs, pos, flags)
}

// Split separates tokens into positional tokens and raw flag values keyed by
// flag name. Flags are `--name value`, `--name=value`, `-s value` and
// `-s=value`; a bool flag takes no separate value (`--fill`, `--fill=false`).
// A token starting with `-` and a digit is a positional (a negative
// coordinate), and every token after `--` is a positional. Unknown flags and
// flags given twice are usage errors.
func Split(specs []ArgSpec, tokens []string) (pos []string, flags map[string]string, err error) {
	long := map[string]ArgSpec{}
	short := map[string]ArgSpec{}
	for _, a := range specs {
		if !a.Flag {
			continue
		}
		long[a.Name] = a
		if a.Short != "" {
			short[a.Short] = a
		}
	}
	flags = map[string]string{}
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t == "--" {
			return append(pos, tokens[i+1:]...), flags, nil
		}
		if !isFlag(t) {
			pos = append(pos, t)
			continue
		}
		var (
			a  ArgSpec
			ok bool
		)
		name, val, hasVal := strings.Cut(strings.TrimPrefix(t[1:], "-"), "=")
		if strings.HasPrefix(t, "--") {
			a, ok = long[name]
		} else {
			a, ok = short[name]
		}
		if !ok {
			return nil, nil, errs.New(errs.Usage, "unknown flag %s", strings.SplitN(t, "=", 2)[0])
		}
		if _, dup := flags[a.Name]; dup {
			return nil, nil, errs.New(errs.Usage, "--%s is given twice", a.Name).WithArg(a.Name)
		}
		if !hasVal && a.Kind != KindBool {
			if i+1 == len(tokens) {
				return nil, nil, errs.New(errs.Usage, "--%s needs a %s value", a.Name, a.Kind).WithArg(a.Name)
			}
			i++
			val = tokens[i]
		} else if !hasVal {
			val = "true"
		}
		flags[a.Name] = val
	}
	return pos, flags, nil
}

func isFlag(t string) bool {
	return len(t) > 1 && t[0] == '-' && !isDigit(t[1])
}

// Build parses positional tokens and raw flag values (as returned by Split)
// into Args. Missing arguments with a Default get the parsed default; other
// missing optional arguments are absent. A KindRect or KindArea positional
// also accepts the two-token form `x,y w,h`: it takes the next token
// whenever that is a size, so specs don't put a point or size right after
// one. The trailing repeat group
// is parsed by whole repetitions (D40). Malformed values, missing required
// positionals and extra positionals are usage errors naming the argument.
// Flags in flags that specs doesn't declare are ignored.
func Build(specs []ArgSpec, pos []string, flags map[string]string) (Args, error) {
	out := Args{}
	for _, a := range specs {
		if !a.Flag {
			continue
		}
		text, ok := flags[a.Name]
		if !ok {
			if a.Default == "" {
				continue
			}
			text = a.Default
		}
		v, n, err := parseValue(a, []string{text})
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, badValue(a, text)
		}
		out[a.Name] = v
	}
	spec := Spec{Args: specs}
	group := spec.group()
	fixed := spec.positionals()
	fixed = fixed[:len(fixed)-len(group)]
	i := 0
	for _, a := range fixed {
		if i == len(pos) {
			if a.Required {
				return nil, missing(a)
			}
			if a.Default != "" {
				v, _, err := parseValue(a, strings.Fields(a.Default))
				if err != nil {
					return nil, err
				}
				out[a.Name] = v
			}
			continue
		}
		v, n, err := parseValue(a, pos[i:])
		if err != nil {
			return nil, err
		}
		out[a.Name] = v
		i += n
	}
	if len(group) > 0 {
		if err := buildGroup(out, group, pos[i:]); err != nil {
			return nil, err
		}
		i = len(pos)
	}
	if i < len(pos) {
		return nil, errs.New(errs.Usage, "unexpected argument %q", pos[i])
	}
	return out, nil
}

// buildGroup parses the trailing repeat group (D40) from pos: whole
// repetitions of every member in order, each member's values in a slice.
// A partial last repetition is an error, and so is an empty required
// group. An empty optional group gets its default, if it has one.
func buildGroup(out Args, group []ArgSpec, pos []string) error {
	if len(pos) == 0 {
		a := group[0]
		if a.Required {
			return missing(a)
		}
		if a.Default != "" {
			v, _, err := parseValue(a, strings.Fields(a.Default))
			if err != nil {
				return err
			}
			out[a.Name] = appendValue(a.Kind, nil, v)
		}
		return nil
	}
	lists := make([]any, len(group))
	for i := 0; i < len(pos); {
		for j, a := range group {
			if i == len(pos) {
				return errs.New(errs.Usage, "missing argument <%s> to complete the last group", a.Name).
					WithHint("%s repeats as a group: %s", a.Name, groupForm(group)).WithArg(a.Name)
			}
			v, n, err := parseValue(a, pos[i:])
			if err != nil {
				return err
			}
			lists[j] = appendValue(a.Kind, lists[j], v)
			i += n
		}
	}
	for j, a := range group {
		out[a.Name] = lists[j]
	}
	return nil
}

func groupForm(group []ArgSpec) string {
	names := make([]string, len(group))
	for i, a := range group {
		names[i] = "<" + a.Name + ">"
	}
	one := strings.Join(names, " ")
	return one + " [" + one + " ...]"
}

func missing(a ArgSpec) error {
	return errs.New(errs.Usage, "missing argument <%s>", a.Name).WithArg(a.Name)
}

// parseValue parses the value of a from the start of tokens and returns how
// many tokens it used: one, or two for the `x,y w,h` rect form.
func parseValue(a ArgSpec, tokens []string) (any, int, error) {
	t := tokens[0]
	switch a.Kind {
	case KindString:
		return t, 1, nil
	case KindColor:
		if t == "" {
			return nil, 0, errs.New(errs.Usage, "empty color for %s", a.Name).WithArg(a.Name)
		}
		return t, 1, nil
	case KindEnum:
		if slices.Contains(a.Enum, t) {
			return t, 1, nil
		}
		return nil, 0, errs.New(errs.Usage, "bad %s %q", a.Name, t).
			WithHint("use one of: %s", strings.Join(a.Enum, ", ")).WithArg(a.Name)
	case KindInt:
		n, err := strconv.Atoi(t)
		if err != nil {
			return nil, 0, badValue(a, t)
		}
		return n, 1, nil
	case KindBool:
		b, err := strconv.ParseBool(t)
		if err != nil {
			return nil, 0, badValue(a, t)
		}
		return b, 1, nil
	case KindPoint:
		n, ok := ints(t, 2)
		if !ok {
			return nil, 0, badValue(a, t)
		}
		return doc.Point{X: n[0], Y: n[1]}, 1, nil
	case KindSize:
		s, ok := parseSize(t)
		if !ok {
			return nil, 0, badValue(a, t)
		}
		return s, 1, nil
	case KindRect:
		if n, ok := ints(t, 4); ok && n[2] > 0 && n[3] > 0 {
			return doc.Rect{X: n[0], Y: n[1], W: n[2], H: n[3]}, 1, nil
		}
		n, ok := ints(t, 2)
		if !ok || a.Flag || len(tokens) < 2 {
			return nil, 0, badValue(a, t)
		}
		s, ok := parseSize(tokens[1])
		if !ok {
			return nil, 0, badValue(a, t+" "+tokens[1])
		}
		return doc.Rect{X: n[0], Y: n[1], W: s.W, H: s.H}, 2, nil
	case KindArea:
		if n, ok := ints(t, 4); ok && n[2] > 0 && n[3] > 0 {
			return doc.Rect{X: n[0], Y: n[1], W: n[2], H: n[3]}, 1, nil
		}
		n, ok := ints(t, 2)
		if !ok {
			return nil, 0, badValue(a, t)
		}
		r := doc.Rect{X: n[0], Y: n[1], W: 1, H: 1}
		if a.Flag || len(tokens) < 2 {
			return r, 1, nil
		}
		// `x,y w,h` when a size follows, otherwise the single pixel x,y.
		if s, ok := parseSize(tokens[1]); ok {
			r.W, r.H = s.W, s.H
			return r, 2, nil
		}
		return r, 1, nil
	}
	// KindGrid: letter grid blocks arrive with grid paint and batch scripts.
	return nil, 0, errs.New(errs.Usage, "grid arguments are not supported yet").WithArg(a.Name)
}

var kindForms = map[ArgKind]string{
	KindInt:   "an integer",
	KindBool:  "true or false",
	KindPoint: "x,y",
	KindSize:  "w,h or wxh, both at least 1",
	KindRect:  "x,y,w,h or x,y w,h, with w and h at least 1",
	KindArea:  "x,y,w,h, x,y w,h, or x,y for one pixel, with w and h at least 1",
}

func badValue(a ArgSpec, t string) error {
	hint := fmt.Sprintf("%s is %s", a.Name, kindForms[a.Kind])
	switch a.Kind {
	case KindPoint, KindSize, KindRect, KindArea:
		hint += fmt.Sprintf(", numbers within -%d..%d", MaxCoord, MaxCoord)
	}
	return errs.New(errs.Usage, "bad %s %q for %s", a.Kind, t, a.Name).WithHint("%s", hint).WithArg(a.Name)
}

// MaxCoord bounds every number in a point, size or rect argument. Shapes
// may reach off the canvas, but not without limit, so drawing stays fast
// and coordinate math can't overflow.
const MaxCoord = 1 << 20

// ints parses exactly n comma-separated integers within ±MaxCoord.
func ints(t string, n int) ([]int, bool) {
	parts := strings.Split(t, ",")
	if len(parts) != n {
		return nil, false
	}
	out := make([]int, n)
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < -MaxCoord || v > MaxCoord {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func parseSize(t string) (Size, bool) {
	n, ok := ints(t, 2)
	if !ok {
		w, h, found := strings.Cut(t, "x")
		if !found {
			return Size{}, false
		}
		if n, ok = ints(w+","+h, 2); !ok {
			return Size{}, false
		}
	}
	if n[0] < 1 || n[1] < 1 {
		return Size{}, false
	}
	return Size{W: n[0], H: n[1]}, true
}

// appendValue appends v to list, a slice of the kind's element type (nil
// for an empty list).
func appendValue(k ArgKind, list, v any) any {
	switch k {
	case KindInt:
		l, _ := list.([]int)
		return append(l, v.(int))
	case KindPoint:
		l, _ := list.([]doc.Point)
		return append(l, v.(doc.Point))
	case KindSize:
		l, _ := list.([]Size)
		return append(l, v.(Size))
	case KindRect, KindArea:
		l, _ := list.([]doc.Rect)
		return append(l, v.(doc.Rect))
	default:
		l, _ := list.([]string)
		return append(l, v.(string))
	}
}

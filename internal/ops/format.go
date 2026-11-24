package ops

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Format returns the canonical script line for a call of the op described
// by spec, such as "rect 6,1,4,3 ink --outline". Positionals come first in
// spec order, then flags in spec order. Arguments that are absent or equal
// to their default are left out (a trailing optional positional only; one
// in the middle keeps its place). Values use their canonical text: points
// `x,y`, sizes `w,h`, rects `x,y,w,h`, and shell-style single quotes for
// strings that need them. Parse(spec.Args, tokens of Format(...)) gives back
// the same Args, with defaults filled in.
func Format(spec Spec, a Args) string {
	var pos []string
	keep := 0 // positional tokens up to the last one that must stay
	for _, s := range spec.Args {
		if s.Flag {
			continue
		}
		v, ok := a[s.Name]
		if !ok {
			break
		}
		var text []string
		if s.Repeat {
			rv := reflect.ValueOf(v)
			for i := range rv.Len() {
				text = append(text, formatValue(s.Kind, rv.Index(i).Interface()))
			}
		} else {
			text = []string{formatValue(s.Kind, v)}
		}
		pos = append(pos, text...)
		if s.Required || !isDefault(s, text) {
			keep = len(pos)
		}
	}
	parts := append([]string{spec.Name}, pos[:keep]...)
	for _, s := range spec.Args {
		v, ok := a[s.Name]
		if !s.Flag || !ok {
			continue
		}
		text := formatValue(s.Kind, v)
		switch {
		case isDefault(s, []string{text}):
		case s.Kind == KindBool && text == "true":
			parts = append(parts, "--"+s.Name)
		case s.Kind == KindBool:
			parts = append(parts, "--"+s.Name+"=false")
		default:
			parts = append(parts, "--"+s.Name, text)
		}
	}
	return strings.Join(parts, " ")
}

// isDefault reports whether text is the canonical text of s's default. A
// bool without a default defaults to false.
func isDefault(s ArgSpec, text []string) bool {
	def := s.Default
	if def == "" {
		if s.Kind != KindBool {
			return false
		}
		def = "false"
	}
	toks := strings.Fields(def)
	v, _, err := parseValue(s, toks)
	if err != nil {
		return false
	}
	return len(text) == 1 && text[0] == formatValue(s.Kind, v)
}

func formatValue(k ArgKind, v any) string {
	switch k {
	case KindInt:
		return strconv.Itoa(v.(int))
	case KindBool:
		return strconv.FormatBool(v.(bool))
	case KindPoint:
		p := v.(doc.Point)
		return strconv.Itoa(p.X) + "," + strconv.Itoa(p.Y)
	case KindSize:
		s := v.(Size)
		return strconv.Itoa(s.W) + "," + strconv.Itoa(s.H)
	case KindRect:
		r := v.(doc.Rect)
		return strconv.Itoa(r.X) + "," + strconv.Itoa(r.Y) + "," + strconv.Itoa(r.W) + "," + strconv.Itoa(r.H)
	}
	return Quote(v.(string))
}

// Quote returns s as one script token: unchanged when it only has
// characters that need no quoting, otherwise in single quotes. A single
// quote inside is closed, escaped with a backslash and reopened, as in a
// shell.
func Quote(s string) string {
	safe := s != "" && s != "#"
	for i := 0; i < len(s) && safe; i++ {
		c := s[i]
		safe = c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_.,:+-#%@/=", c) >= 0
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

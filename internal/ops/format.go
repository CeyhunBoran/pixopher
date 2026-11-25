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
	group := spec.group()
	fixed := spec.positionals()
	fixed = fixed[:len(fixed)-len(group)]
	for _, s := range fixed {
		v, ok := a[s.Name]
		if !ok {
			break
		}
		text := formatValue(s.Kind, v)
		pos = append(pos, text)
		if s.Required || !isDefault(s, []string{text}) {
			keep = len(pos)
		}
	}
	if len(pos) == len(fixed) && len(group) > 0 {
		// Repetitions are interleaved: px 1,1 ink 2,2 red (D40).
		lists := make([]reflect.Value, len(group))
		n := -1
		for j, s := range group {
			if v, ok := a[s.Name]; ok {
				lists[j] = reflect.ValueOf(v)
				if n < 0 || lists[j].Len() < n {
					n = lists[j].Len()
				}
			} else {
				n = 0
			}
		}
		var text []string
		for i := range max(n, 0) {
			for j, s := range group {
				text = append(text, formatValue(s.Kind, lists[j].Index(i).Interface()))
			}
		}
		pos = append(pos, text...)
		if len(text) > 0 && (group[0].Required || len(group) > 1 || !isDefault(group[0], text)) {
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
	case KindArea:
		r := v.(doc.Rect)
		if r.W == 1 && r.H == 1 {
			return strconv.Itoa(r.X) + "," + strconv.Itoa(r.Y)
		}
		return formatValue(KindRect, r)
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

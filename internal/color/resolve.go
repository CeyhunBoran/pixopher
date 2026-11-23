package color

import (
	"errors"
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// Resolved is the result of resolving a color argument.
type Resolved struct {
	// Index is the palette entry, valid when InPalette is true.
	Index doc.Index
	// InPalette is false only for a raw hex color that matches no entry. The
	// caller then adds Palette.AutoEntry(Color, ...) as part of its change.
	InPalette bool
	// Color is the entry's color, or the raw hex color.
	Color doc.RGBA
}

// Resolve interprets s using the color syntax from docs/palettes.md: palette
// key, palette name, ramp name, ramp step (skin:0, skin:-1), ramp relative
// (skin+1, skin-1), hex, or transparent (".", "transparent", "none").
// key: and name: prefixes force one interpretation.
//
// A string as long as the document's key width is a key first. A word is a
// palette entry name first, then a ramp name. A hex color that matches an
// entry exactly resolves to that entry, and any hex with alpha 0 resolves to
// transparent. Resolve never modifies d.
func Resolve(d *doc.Doc, s string) (Resolved, error) {
	r, err := resolve(d, s)
	if e, ok := errors.AsType[*errs.Error](err); ok && e.Arg == "" {
		e.Arg = "color"
	}
	return r, err
}

func resolve(d *doc.Doc, s string) (Resolved, error) {
	p := &d.Palette
	entry := func(i doc.Index) (Resolved, error) {
		return Resolved{Index: i, InPalette: true, Color: p.Entries[i].Color}, nil
	}
	switch {
	case s == "":
		return Resolved{}, errs.New(errs.InvalidColor, "empty color")
	case s == "." || s == doc.TransparentKey(d.KeyWidth) || s == "transparent" || s == "none":
		return entry(doc.Transparent)
	case strings.HasPrefix(s, "key:"):
		k := s[len("key:"):]
		if i, ok := p.ByKey(k); ok {
			return entry(i)
		}
		return Resolved{}, unknown("no palette key %q", k, keyCandidates(p))
	case strings.HasPrefix(s, "name:"):
		n := s[len("name:"):]
		if i, ok := p.ByName(n); ok {
			return entry(i)
		}
		return Resolved{}, unknown("no palette entry named %q", n, nameCandidates(p))
	case s[0] == '#':
		c, err := ParseHex(s)
		if err != nil {
			return Resolved{}, err
		}
		if c.A == 0 {
			return entry(doc.Transparent)
		}
		if i, ok := p.ByColor(c); ok {
			return entry(i)
		}
		return Resolved{Color: c}, nil
	}
	if len(s) == d.KeyWidth {
		if i, ok := p.ByKey(s); ok {
			return entry(i)
		}
	}
	if i := strings.IndexAny(s, ":+-"); i > 0 {
		return rampStep(d, s, s[:i], s[i], s[i+1:])
	}
	if i, ok := p.ByName(s); ok {
		return entry(i)
	}
	if r, ok := d.Ramps[s]; ok {
		return entry(r.Steps[r.Base])
	}
	if !doc.ValidName(s) && !doc.ValidKey(s, d.KeyWidth) {
		return Resolved{}, errs.New(errs.InvalidColor, "invalid color %q", s).
			WithHint("use a palette key or name, a ramp (skin, skin:0, skin+1), #rrggbb, or transparent")
	}
	cands := append(nameCandidates(p), rampCandidates(d)...)
	return Resolved{}, unknown("unknown color %q", s, cands)
}

// rampStep resolves "ramp:n" (absolute, negative from the light end) and
// "ramp+n" / "ramp-n" (relative to the ramp's base step).
func rampStep(d *doc.Doc, s, name string, op byte, num string) (Resolved, error) {
	r, ok := d.Ramps[name]
	if !ok {
		return Resolved{}, unknown("no ramp named %q", name, rampCandidates(d))
	}
	n, err := strconv.Atoi(num)
	if err != nil || num == "" || num[0] == '+' || num[0] == '-' && op != ':' {
		return Resolved{}, errs.New(errs.InvalidColor, "invalid ramp step %q", s).
			WithHint("use %s:0, %s:-1, %s+1 or %s-1", name, name, name, name)
	}
	steps := len(r.Steps)
	var i int
	switch op {
	case ':':
		i = n
		if n < 0 {
			i = steps + n
		}
	case '+':
		i = r.Base + n
	case '-':
		i = r.Base - n
	}
	if i < 0 || i >= steps {
		return Resolved{}, errs.New(errs.InvalidColor, "ramp step %q is out of range", s).
			WithHint("ramp %q has %d steps (0..%d, base %d)", name, steps, steps-1, r.Base)
	}
	idx := r.Steps[i]
	return Resolved{Index: idx, InPalette: true, Color: d.Palette.Entries[idx].Color}, nil
}

func unknown(format, what string, cands []string) *errs.Error {
	e := errs.New(errs.UnknownColor, format, what)
	if best := Closest(what, cands); best != "" {
		return e.WithHint("did you mean %q?", best)
	}
	return e
}

func keyCandidates(p *doc.Palette) []string {
	out := make([]string, 0, p.Len())
	for _, e := range p.Entries[1:] {
		out = append(out, e.Key)
	}
	return out
}

func nameCandidates(p *doc.Palette) []string {
	out := make([]string, 0, p.Len())
	for _, e := range p.Entries[1:] {
		out = append(out, e.Name)
	}
	return out
}

func rampCandidates(d *doc.Doc) []string { return doc.SortedKeys(d.Ramps) }

// Closest returns the candidate nearest to s by edit distance, if it is close
// enough to be a likely typo (at most 2 edits, and fewer than len(s)). Ties go
// to the earliest candidate, so the result is deterministic.
func Closest(s string, cands []string) string {
	best, bestD := "", 3
	for _, c := range cands {
		if c == s {
			continue
		}
		if dist := levenshtein(s, c); dist < bestD && dist < len(s) {
			best, bestD = c, dist
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

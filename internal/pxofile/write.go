package pxofile

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Serialize writes d in canonical form: sections and fixed fields in the order
// of docs/file-format.md, name-keyed tables sorted (palette order is kept),
// defaults and empty sections omitted, LF line endings and a final newline. The same
// document always gives the same bytes.
//
// d must be valid (see doc.Validate); Serialize returns its error otherwise.
func Serialize(d *doc.Doc) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	w := &writer{d: d}
	w.header()
	w.meta()
	w.palette()
	w.ramps()
	w.symmetry()
	w.layers()
	w.frames()
	w.tags()
	w.markers()
	w.variants()
	w.lint()
	w.parts()
	w.cels()
	return w.buf.Bytes(), nil
}

type writer struct {
	d   *doc.Doc
	buf bytes.Buffer
}

func (w *writer) printf(format string, a ...any) { fmt.Fprintf(&w.buf, format, a...) }

// section starts a table, separated from what came before by a blank line.
func (w *writer) section(header string) { w.printf("\n%s\n", header) }

func (w *writer) header() {
	d := w.d
	w.printf("pixopher = %d\nsize = [%d, %d]\nrev = %d\nkey_width = %d\n", d.Version, d.W, d.H, d.Rev, d.KeyWidth)
}

func (w *writer) meta() {
	m := w.d.Meta
	if m.Title == "" && m.Created.IsZero() {
		return
	}
	w.section("[meta]")
	if m.Title != "" {
		w.printf("title = %s\n", quote(m.Title))
	}
	if !m.Created.IsZero() {
		w.printf("created = %s\n", m.Created.UTC().Format(time.RFC3339Nano))
	}
}

func (w *writer) palette() {
	w.section("[palette]")
	for _, e := range w.d.Palette.Entries {
		w.printf("%s = [%s, %s]\n", quoteKey(e.Key), quote(color.Hex(e.Color)), quote(e.Name))
	}
}

func (w *writer) keyList(steps []doc.Index) string {
	ks := make([]string, len(steps))
	for i, s := range steps {
		ks[i] = quote(w.d.Palette.Entries[s].Key)
	}
	return "[" + strings.Join(ks, ", ") + "]"
}

func (w *writer) ramps() {
	if len(w.d.Ramps) == 0 {
		return
	}
	w.section("[ramps]")
	for _, name := range doc.SortedKeys(w.d.Ramps) {
		r := w.d.Ramps[name]
		if r.Base == doc.DefaultBase(len(r.Steps)) {
			w.printf("%s = %s\n", quoteKey(name), w.keyList(r.Steps))
			continue
		}
		base := w.d.Palette.Entries[r.Steps[r.Base]].Key
		w.printf("%s = { steps = %s, base = %s }\n", quoteKey(name), w.keyList(r.Steps), quote(base))
	}
}

func axis(a float64) string { return strconv.FormatFloat(a, 'f', 1, 64) }

func (w *writer) symmetry() {
	s, def := w.d.Symmetry, doc.DefaultSymmetry(w.d.W, w.d.H)
	if s == def {
		return
	}
	w.section("[symmetry]")
	w.printf("mode = %s\n", quote(s.Mode))
	if s.AxisX != def.AxisX {
		w.printf("axis_x = %s\n", axis(s.AxisX))
	}
	if s.AxisY != def.AxisY {
		w.printf("axis_y = %s\n", axis(s.AxisY))
	}
}

func (w *writer) layers() {
	for _, l := range w.d.Layers {
		w.section("[[layers]]")
		w.printf("name = %s\n", quote(l.Name))
		if !l.Visible {
			w.printf("visible = false\n")
		}
		if l.Locked {
			w.printf("locked = true\n")
		}
		if l.Opacity != doc.DefaultOpacity {
			w.printf("opacity = %d\n", l.Opacity)
		}
		if l.Blend != doc.DefaultBlend {
			w.printf("blend = %s\n", quote(l.Blend))
		}
	}
}

func point(p doc.Point) string { return fmt.Sprintf("[%d, %d]", p.X, p.Y) }

func (w *writer) frames() {
	for _, f := range w.d.Frames {
		w.section("[[frames]]")
		w.printf("duration_ms = %d\n", f.DurationMS)
		if len(f.Anchors) > 0 {
			parts := make([]string, 0, len(f.Anchors))
			for _, name := range doc.SortedKeys(f.Anchors) {
				parts = append(parts, quoteKey(name)+" = "+point(f.Anchors[name]))
			}
			w.printf("anchors = { %s }\n", strings.Join(parts, ", "))
		}
	}
}

func (w *writer) tags() {
	for _, t := range w.d.Tags {
		w.section("[[tags]]")
		w.printf("name = %s\nfrom = %d\nto = %d\n", quote(t.Name), t.From, t.To)
		if t.Direction != doc.DefaultDirection {
			w.printf("direction = %s\n", quote(t.Direction))
		}
	}
}

func (w *writer) markers() {
	if len(w.d.Regions) > 0 {
		w.section("[regions]")
		for _, name := range doc.SortedKeys(w.d.Regions) {
			r := w.d.Regions[name]
			w.printf("%s = [%d, %d, %d, %d]\n", quoteKey(name), r.X, r.Y, r.W, r.H)
		}
	}
	if len(w.d.Anchors) > 0 {
		w.section("[anchors]")
		for _, name := range doc.SortedKeys(w.d.Anchors) {
			w.printf("%s = %s\n", quoteKey(name), point(w.d.Anchors[name]))
		}
	}
}

func (w *writer) variants() {
	for _, name := range doc.SortedKeys(w.d.Variants) {
		v := w.d.Variants[name]
		w.section("[variants." + quoteKey(name) + "]")
		// Ramp and entry overrides share one table, sorted together. A name
		// can't be both in one variant: TOML keys are unique.
		lines := map[string]string{}
		for r, cs := range v.Ramps {
			hs := make([]string, len(cs))
			for i, c := range cs {
				hs[i] = quote(color.Hex(c))
			}
			lines[r] = "[" + strings.Join(hs, ", ") + "]"
		}
		for e, c := range v.Entries {
			lines[e] = quote(color.Hex(c))
		}
		for _, k := range doc.SortedKeys(lines) {
			w.printf("%s = %s\n", quoteKey(k), lines[k])
		}
	}
}

func (w *writer) lint() {
	l := w.d.Lint
	if l.MaxColors == 0 && len(l.Disable) == 0 {
		return
	}
	w.section("[lint]")
	if l.MaxColors > 0 {
		w.printf("max_colors = %d\n", l.MaxColors)
	}
	if len(l.Disable) > 0 {
		qs := make([]string, len(l.Disable))
		for i, r := range l.Disable {
			qs[i] = quote(r)
		}
		w.printf("disable = [%s]\n", strings.Join(qs, ", "))
	}
}

func (w *writer) keys() []string {
	ks := make([]string, w.d.Palette.Len())
	for i, e := range w.d.Palette.Entries {
		ks[i] = e.Key
	}
	return ks
}

func (w *writer) grid(e encoded) {
	if !e.RLE {
		w.printf("grid = '''\n%s'''\n", e.Grid)
		return
	}
	w.printf("encoding = \"rle\"\nrows = [\n")
	for _, r := range e.Rows {
		w.printf("  %s,\n", quote(r))
	}
	w.printf("]\n")
}

func (w *writer) parts() {
	keys := w.keys()
	for _, name := range doc.SortedKeys(w.d.Parts) {
		p := w.d.Parts[name]
		w.section("[parts." + quoteKey(name) + "]")
		w.grid(encode(p.Pix, p.W, p.H, keys))
	}
}

// cels writes cels sorted by layer, then frame, each cropped to its content.
// Empty cels, and links to them, are left out: a missing cel means empty.
func (w *writer) cels() {
	d, keys := w.d, w.keys()
	for _, k := range d.CelKeys() {
		c := d.Cels[k]
		if c.Linked {
			if t := d.Cels[doc.CelKey{Layer: k.Layer, Frame: c.LinkFrame}]; t.Empty() {
				continue
			}
		}
		if c.Linked {
			w.celHeader(k)
			w.printf("link = { frame = %d }\n", c.LinkFrame)
			continue
		}
		box, ok := bbox(c.Pix, d.W, d.H)
		if !ok {
			continue
		}
		w.celHeader(k)
		w.printf("origin = [%d, %d]\n", box.X, box.Y)
		crop := make([]doc.Index, 0, box.W*box.H)
		for y := box.Y; y < box.Y+box.H; y++ {
			crop = append(crop, c.Pix[y*d.W+box.X:y*d.W+box.X+box.W]...)
		}
		w.grid(encode(crop, box.W, box.H, keys))
	}
}

func (w *writer) celHeader(k doc.CelKey) {
	w.section("[[cels]]")
	w.printf("layer = %s\nframe = %d\n", quote(w.d.Layers[k.Layer].Name), k.Frame)
}

// bbox returns the bounding box of the non-transparent pixels in pix.
func bbox(pix []doc.Index, w, h int) (doc.Rect, bool) {
	x0, y0, x1, y1 := w, h, -1, -1
	for y := range h {
		for x, p := range pix[y*w : (y+1)*w] {
			if p != doc.Transparent {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return doc.Rect{}, false
	}
	return doc.Rect{X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1}, true
}

// quote returns s as a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// quoteKey returns k as a TOML key: bare when it only uses A-Za-z0-9_-,
// quoted otherwise.
func quoteKey(k string) string {
	if k == "" || strings.ContainsFunc(k, func(r rune) bool { return !isBareKeyRune(r) }) {
		return quote(k)
	}
	return k
}

func isBareKeyRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-'
}

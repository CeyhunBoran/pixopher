package ops

import (
	"fmt"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

type infoOp struct{}

func (infoOp) Spec() Spec {
	return Spec{
		Name: "info", Group: "document",
		Summary:  "Show the document's size, rev, palette size, layers, frames, regions and anchors",
		Examples: []string{"info", "info hero.pxo --json"},
	}
}

// InfoReport is the result of info. Its JSON form is the --json output of
// `pxo info` (docs/cli.md#document, D52).
type InfoReport struct {
	Rev      int               `json:"rev"`
	Size     [2]int            `json:"size"`      // [w, h]
	KeyWidth int               `json:"key_width"` // characters per pixel in grids
	Palette  int               `json:"palette"`   // entries other than transparent
	Layers   []InfoLayer       `json:"layers"`    // bottom first
	Frames   []InfoFrame       `json:"frames"`
	Regions  map[string][4]int `json:"regions"` // name to [x, y, w, h]
	Anchors  map[string][2]int `json:"anchors"` // name to [x, y], default positions
}

// InfoLayer is one layer of an InfoReport.
type InfoLayer struct {
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
	Locked  bool   `json:"locked"`
	Opacity int    `json:"opacity"`
	Blend   string `json:"blend"`
}

// InfoFrame is one frame of an InfoReport.
type InfoFrame struct {
	DurationMS int `json:"duration_ms"`
}

// Observe reports the document's structure. It never reads pixels.
func (infoOp) Observe(d *doc.Doc, _ Args) (Report, error) {
	r := InfoReport{
		Rev: d.Rev, Size: [2]int{d.W, d.H}, KeyWidth: d.KeyWidth, Palette: d.Palette.Len() - 1,
		Layers:  make([]InfoLayer, len(d.Layers)),
		Frames:  make([]InfoFrame, len(d.Frames)),
		Regions: make(map[string][4]int, len(d.Regions)),
		Anchors: make(map[string][2]int, len(d.Anchors)),
	}
	for i, l := range d.Layers {
		r.Layers[i] = InfoLayer{Name: l.Name, Visible: l.Visible, Locked: l.Locked, Opacity: int(l.Opacity), Blend: l.Blend}
	}
	for i, f := range d.Frames {
		r.Frames[i] = InfoFrame{DurationMS: f.DurationMS}
	}
	for name, v := range d.Regions {
		r.Regions[name] = [4]int{v.X, v.Y, v.W, v.H}
	}
	for name, p := range d.Anchors {
		r.Anchors[name] = [2]int{p.X, p.Y}
	}
	return &r, nil
}

// Text is the human form, one labelled line per item:
//
//	size     16x16
//	rev      12
//	palette  6 colors, key width 1
//	layers   body, shine (hidden)
//	frames   1 (120 ms)
//	regions  flask 3,4 10,11; neck 6,1 4,3
//	anchors  cork 8,1
func (r *InfoReport) Text() string {
	var b strings.Builder
	line := func(label, value string) { fmt.Fprintf(&b, "%-8s %s\n", label, value) }
	line("size", fmt.Sprintf("%dx%d", r.Size[0], r.Size[1]))
	line("rev", fmt.Sprint(r.Rev))
	colors := "colors"
	if r.Palette == 1 {
		colors = "color"
	}
	line("palette", fmt.Sprintf("%d %s, key width %d", r.Palette, colors, r.KeyWidth))

	layers := make([]string, len(r.Layers))
	for i, l := range r.Layers {
		var notes []string
		if !l.Visible {
			notes = append(notes, "hidden")
		}
		if l.Locked {
			notes = append(notes, "locked")
		}
		if l.Opacity != doc.DefaultOpacity {
			notes = append(notes, fmt.Sprintf("opacity %d", l.Opacity))
		}
		if l.Blend != doc.DefaultBlend {
			notes = append(notes, l.Blend)
		}
		layers[i] = l.Name
		if len(notes) > 0 {
			layers[i] += " (" + strings.Join(notes, ", ") + ")"
		}
	}
	line("layers", strings.Join(layers, ", "))

	durations := make([]string, len(r.Frames))
	for i, f := range r.Frames {
		durations[i] = fmt.Sprint(f.DurationMS)
	}
	line("frames", fmt.Sprintf("%d (%s ms)", len(r.Frames), strings.Join(durations, ", ")))

	regions := make([]string, 0, len(r.Regions))
	for _, name := range doc.SortedKeys(r.Regions) {
		v := r.Regions[name]
		regions = append(regions, fmt.Sprintf("%s %d,%d %d,%d", name, v[0], v[1], v[2], v[3]))
	}
	line("regions", orNone(strings.Join(regions, "; ")))
	anchors := make([]string, 0, len(r.Anchors))
	for _, name := range doc.SortedKeys(r.Anchors) {
		p := r.Anchors[name]
		anchors = append(anchors, fmt.Sprintf("%s %d,%d", name, p[0], p[1]))
	}
	line("anchors", orNone(strings.Join(anchors, "; ")))
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

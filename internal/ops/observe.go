package ops

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
)

func init() {
	for _, op := range []Op{showOp{}, getOp{}, statsOp{}} {
		Default.Register(op)
	}
}

const (
	// ShowCropOver is the canvas side above which show crops to the content
	// bbox unless --full is given (D55).
	ShowCropOver = 64
	// ShowTileWidth is the most pixels per printed row of show; wider
	// output is printed in tiles of this many columns (D55).
	ShowTileWidth = 128
)

// targetPixels returns the target cel's pixels, following a link, or nil
// when the cel is empty.
func targetPixels(d *doc.Doc) []doc.Index {
	layer, frame := Target(d)
	return d.Pixels(layer, frame)
}

// pixelAt reads pix (from targetPixels) at (x, y), which must be on the
// canvas.
func pixelAt(d *doc.Doc, pix []doc.Index, x, y int) doc.Index {
	if pix == nil {
		return doc.Transparent
	}
	return pix[y*d.W+x]
}

// contentBBox returns the bbox of the non-transparent pixels of pix, and
// false when there are none.
func contentBBox(d *doc.Doc, pix []doc.Index) (doc.Rect, bool) {
	x0, y0, x1, y1 := d.W, d.H, -1, -1
	for i, v := range pix {
		if v == doc.Transparent {
			continue
		}
		x, y := i%d.W, i/d.W
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if x1 < 0 {
		return doc.Rect{}, false
	}
	return doc.Rect{X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1}, true
}

type showOp struct{}

func (showOp) Spec() Spec {
	return Spec{
		Name: "show", Group: "observe",
		Summary: "Print the canvas as a grid of palette keys",
		Args: []ArgSpec{
			{Name: "grid", Kind: KindBool, Flag: true, Help: "print palette keys, one per pixel (the default)"},
			{Name: "rulers", Kind: KindBool, Flag: true, Help: "number the columns and rows"},
			{Name: "legend", Kind: KindBool, Flag: true, Help: "end with the name and color of each key shown"},
			{Name: "full", Kind: KindBool, Flag: true, Help: fmt.Sprintf("don't crop canvases over %d px to their content", ShowCropOver)},
		},
		Examples: []string{"show --rulers --legend", "show --json"},
	}
}

// ShowReport is the result of show. Its JSON form is the --json output of
// `pxo show` (docs/cli.md#observation-non-mutating, D55). --rulers and
// --legend only change the human form.
type ShowReport struct {
	Origin [2]int                 `json:"origin"` // canvas position of the first key of rows[0]
	Rows   []string               `json:"rows"`   // one string of keys per row, top first
	Legend map[string]LegendEntry `json:"legend"` // every key in rows

	keyWidth       int
	rulers, legend bool
	order          []string // legend keys in palette order
}

// LegendEntry is the name and color of one key of a ShowReport.
type LegendEntry struct {
	Name  string `json:"name"`
	Color string `json:"color"` // #rrggbb, or #rrggbbaa when not opaque
}

// Observe prints the target cel. Canvases with a side over ShowCropOver
// are cropped to their content unless --full is given; an empty one is
// shown whole.
func (showOp) Observe(d *doc.Doc, args Args) (Report, error) {
	pix := targetPixels(d)
	area := doc.Rect{W: d.W, H: d.H}
	if !args.Bool("full") && (d.W > ShowCropOver || d.H > ShowCropOver) {
		if bb, ok := contentBBox(d, pix); ok {
			area = bb
		}
	}
	r := ShowReport{
		Origin: [2]int{area.X, area.Y}, Rows: make([]string, area.H), Legend: map[string]LegendEntry{},
		keyWidth: d.KeyWidth, rulers: args.Bool("rulers"), legend: args.Bool("legend"),
	}
	used := make([]bool, d.Palette.Len())
	var b strings.Builder
	for y := range area.H {
		b.Reset()
		for x := range area.W {
			v := pixelAt(d, pix, area.X+x, area.Y+y)
			used[v] = true
			b.WriteString(d.Palette.Entries[v].Key)
		}
		r.Rows[y] = b.String()
	}
	for i, ok := range used {
		if ok {
			e := d.Palette.Entries[i]
			r.Legend[e.Key] = LegendEntry{Name: e.Name, Color: color.Hex(e.Color)}
			r.order = append(r.order, e.Key)
		}
	}
	return &r, nil
}

// Text is the human form: the rows, in tiles of ShowTileWidth columns
// separated by a blank line, then the legend. With --rulers, each tile
// starts with one line per digit of the largest x shown (a digit where
// it changes, and at the tile's first column), and each row with its y:
//
//	           1
//	   0123456789012345
//	 0 ....oooo........
//	legend: . transparent  o ink #1a1c2c
func (r *ShowReport) Text() string {
	var b strings.Builder
	h := len(r.Rows)
	w := 0
	if h > 0 {
		w = len(r.Rows[0]) / r.keyWidth
	}
	x0, y0 := r.Origin[0], r.Origin[1]
	gutter := len(strconv.Itoa(y0 + h - 1))
	for t := 0; t < w; t += ShowTileWidth {
		end := min(t+ShowTileWidth, w)
		if t > 0 {
			b.WriteByte('\n')
		}
		if r.rulers {
			digits := len(strconv.Itoa(x0 + w - 1))
			for k, p := digits-1, int(math.Pow10(digits-1)); k >= 0; k, p = k-1, p/10 {
				var line strings.Builder
				line.WriteString(strings.Repeat(" ", gutter+2))
				for x := x0 + t; x < x0+end; x++ {
					line.WriteString(strings.Repeat(" ", r.keyWidth-1))
					if k == 0 || x%p == 0 || x == x0+t {
						line.WriteByte(byte('0' + x/p%10))
					} else {
						line.WriteByte(' ')
					}
				}
				b.WriteString(strings.TrimRight(line.String(), " ") + "\n")
			}
		}
		for y, row := range r.Rows {
			if r.rulers {
				fmt.Fprintf(&b, "%*d  ", gutter, y0+y)
			}
			b.WriteString(row[t*r.keyWidth:end*r.keyWidth] + "\n")
		}
	}
	if r.legend {
		parts := make([]string, len(r.order))
		for i, k := range r.order {
			e := r.Legend[k]
			if e.Name == doc.TransparentName {
				parts[i] = k + " " + e.Name
			} else {
				parts[i] = k + " " + e.Name + " " + e.Color
			}
		}
		b.WriteString("legend: " + strings.Join(parts, "  ") + "\n")
	}
	return b.String()
}

type getOp struct{}

func (getOp) Spec() Spec {
	return Spec{
		Name: "get", Group: "observe",
		Summary: "Print the key, color and name at one pixel",
		Args: []ArgSpec{
			{Name: "at", Kind: KindPoint, Required: true, Help: "the pixel"},
		},
		Examples: []string{"get 5,3", "get 5,3 --json"},
	}
}

// GetReport is the result of get. Its JSON form is the --json output of
// `pxo get` (docs/cli.md#observation-non-mutating, D55).
type GetReport struct {
	At    [2]int `json:"at"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"` // #rrggbb, or #rrggbbaa when not opaque
}

// Observe reads one pixel of the target cel.
func (getOp) Observe(d *doc.Doc, args Args) (Report, error) {
	p := args.Point("at")
	if err := inCanvas(d, "at", p); err != nil {
		return nil, err
	}
	e := d.Palette.Entries[pixelAt(d, targetPixels(d), p.X, p.Y)]
	return &GetReport{At: [2]int{p.X, p.Y}, Key: e.Key, Name: e.Name, Color: color.Hex(e.Color)}, nil
}

// Text is the human form, like a line of palette list: `o  #1a1c2c  ink`.
func (r *GetReport) Text() string { return r.Key + "  " + r.Color + "  " + r.Name + "\n" }

type statsOp struct{}

func (statsOp) Spec() Spec {
	return Spec{
		Name: "stats", Group: "observe",
		Summary:  "Print the filled pixel count, coverage, content bbox and the count of each color",
		Examples: []string{"stats", "stats --json"},
	}
}

// StatsReport is the result of stats. Its JSON form is the --json output
// of `pxo stats` (docs/cli.md#observation-non-mutating, D55).
type StatsReport struct {
	Size     [2]int       `json:"size"`
	Filled   int          `json:"filled"`         // non-transparent pixels
	Coverage float64      `json:"coverage"`       // filled / (w*h), rounded to 4 decimals
	BBox     *[4]int      `json:"bbox,omitempty"` // [x, y, w, h] of the filled pixels; omitted when none
	Colors   []StatsColor `json:"colors"`         // used entries in palette order, without transparent
}

// StatsColor is one used palette entry of a StatsReport.
type StatsColor struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

// Observe counts the target cel's pixels.
func (statsOp) Observe(d *doc.Doc, _ Args) (Report, error) {
	pix := targetPixels(d)
	counts := make([]int, d.Palette.Len())
	for _, v := range pix {
		counts[v]++
	}
	r := StatsReport{Size: [2]int{d.W, d.H}, Colors: []StatsColor{}}
	for i := 1; i < len(counts); i++ {
		if counts[i] > 0 {
			e := d.Palette.Entries[i]
			r.Colors = append(r.Colors, StatsColor{Key: e.Key, Name: e.Name, Color: color.Hex(e.Color), Count: counts[i]})
			r.Filled += counts[i]
		}
	}
	r.Coverage = math.Round(float64(r.Filled)/float64(d.W*d.H)*1e4) / 1e4
	if bb, ok := contentBBox(d, pix); ok {
		r.BBox = &[4]int{bb.X, bb.Y, bb.W, bb.H}
	}
	return &r, nil
}

// Text is the human form: labelled lines, then one line per color with
// its count.
//
//	size     16x16
//	filled   30 of 256 px (11.72%)
//	bbox     4,0 8,5
//	colors   2
//	  20  o  #1a1c2c  ink
//	  10  g  #38b764  glass
func (r *StatsReport) Text() string {
	var b strings.Builder
	line := func(label, value string) { fmt.Fprintf(&b, "%-8s %s\n", label, value) }
	line("size", fmt.Sprintf("%dx%d", r.Size[0], r.Size[1]))
	line("filled", fmt.Sprintf("%d of %d px (%.2f%%)", r.Filled, r.Size[0]*r.Size[1], r.Coverage*100))
	bbox := "none"
	if r.BBox != nil {
		bbox = fmt.Sprintf("%d,%d %d,%d", r.BBox[0], r.BBox[1], r.BBox[2], r.BBox[3])
	}
	line("bbox", bbox)
	line("colors", strconv.Itoa(len(r.Colors)))
	nw, cw := 0, 0
	for _, c := range r.Colors {
		nw, cw = max(nw, len(strconv.Itoa(c.Count))), max(cw, len(c.Color))
	}
	for _, c := range r.Colors {
		fmt.Fprintf(&b, "  %*d  %s  %-*s  %s\n", nw, c.Count, c.Key, cw, c.Color, c.Name)
	}
	return b.String()
}

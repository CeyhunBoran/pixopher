package ops

import (
	"maps"
	"slices"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

func init() { Default.Register(gridOp{}) }

// GridSep separates the rows of a grid written as one token
// (`..oo..,.oRRo.`). It is reserved, so it is never part of a key (D64).
const GridSep = ","

// GridText turns a letter grid written one row per line, as on stdin or in
// a script block, into its one-token form: each row trimmed of spaces,
// tabs and a trailing \r, blank lines before the first row and after the
// last dropped, and the rows joined by GridSep. A row with a GridSep in it
// is an error, as is a grid with no rows.
func GridText(src string) (string, error) {
	rows := strings.Split(src, "\n")
	for i, r := range rows {
		rows[i] = strings.TrimSpace(strings.TrimSuffix(r, "\r"))
	}
	for len(rows) > 0 && rows[0] == "" {
		rows = rows[1:]
	}
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) == 0 {
		return "", emptyGrid()
	}
	for i, r := range rows {
		if strings.Contains(r, GridSep) {
			return "", errs.New(errs.InvalidColor, "row %d has a %q, which is never a palette key", i+1, GridSep).
				WithArg("grid").WithDetails(gridPos{Row: i + 1})
		}
	}
	return strings.Join(rows, GridSep), nil
}

func emptyGrid() *errs.Error {
	return errs.New(errs.Usage, "empty grid").WithArg("grid").
		WithHint("pass the rows on stdin (a heredoc), in a script block between << and >>, or as one token with rows joined by commas")
}

// gridPos is the details of a grid error: the 1-based row, and the 1-based
// column (in cells) when one cell is at fault (D64).
type gridPos struct {
	Row int `json:"row"`
	Col int `json:"col,omitempty"`
}

type gridOp struct{}

func (gridOp) Spec() Spec {
	return Spec{
		Name: "grid", Group: "draw", Summary: "Paint a letter grid of palette keys with its top-left at a point", Mutates: true,
		Args: []ArgSpec{
			{Name: "at", Kind: KindPoint, Required: true, Help: "top-left of the grid; the grid may reach off the canvas"},
			{Name: "grid", Kind: KindGrid, Required: true, Help: "rows joined by commas, or one row per line on stdin when not given; . leaves a pixel alone, _ erases it"},
			{Name: "legend", Kind: KindString, Flag: true, Help: `letters for this call only, such as "s=skin S=skin_dark"`},
			{Name: "opaque", Kind: KindBool, Flag: true, Help: ". erases instead of leaving the pixel alone"},
		},
		Examples: []string{"grid 0,0 .oo.,oRRo,.oo.", "grid 4,2 sss,sSs --legend 's=skin S=skin_dark'", "grid 2,2 o_o,_o_ --opaque"},
	}
}

// cell is what one grid cell does: nothing, write an index, or paint with
// a legend brush (which may add an auto entry).
type cell struct {
	skip  bool
	index doc.Index
	brush *brush
}

func (gridOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	d := ctx.Doc
	kw := d.KeyWidth
	legend, err := parseLegend(ctx, args.String("legend"))
	if err != nil {
		return err
	}
	rows := strings.Split(args.String("grid"), GridSep)
	if len(rows) == 1 && rows[0] == "" {
		return emptyGrid()
	}
	// The shape is checked before any cell, so a ragged grid always reports
	// its row, whatever its cells hold.
	for i, r := range rows {
		if len(r)%kw != 0 {
			return errs.New(errs.GridMismatch, "row %d is %d characters, not a whole number of %d-character cells", i+1, len(r), kw).
				WithArg("grid").WithDetails(gridPos{Row: i + 1}).
				WithHint("this document's keys are %d characters wide", kw)
		}
		if len(r) != len(rows[0]) {
			return errs.New(errs.GridMismatch, "row %d has %d cells, but row 1 has %d", i+1, len(r)/kw, len(rows[0])/kw).
				WithArg("grid").WithDetails(gridPos{Row: i + 1}).
				WithHint("every row must have the same width; use . for pixels to leave alone")
		}
	}
	if len(rows[0]) == 0 {
		return emptyGrid()
	}
	transparent, erase := doc.TransparentKey(kw), doc.EraseKey(kw)
	cells := make([][]cell, len(rows))
	for y, r := range rows {
		cells[y] = make([]cell, len(r)/kw)
		for x := range cells[y] {
			k := r[x*kw : (x+1)*kw]
			c, err := gridCell(d, legend, k, transparent, erase, args.Bool("opaque"))
			if err != nil {
				return err.WithArg("grid").WithDetails(gridPos{Row: y + 1, Col: x + 1})
			}
			cells[y][x] = c
		}
	}
	at := args.Point("at")
	for y, row := range cells {
		for x, c := range row {
			px, py := at.X+x, at.Y+y
			switch {
			case c.skip || !ctx.In(px, py):
			case c.brush != nil:
				// Legend brushes add their auto entry on their first pixel
				// on the canvas, in row-major order (D48).
				if c.brush.set(px, py); c.brush.err != nil {
					return c.brush.err
				}
			default:
				ctx.Set(px, py, c.index)
			}
		}
	}
	return nil
}

// gridCell resolves one cell's key: a legend letter first, then the
// transparent and erase markers, then a palette key.
func gridCell(d *doc.Doc, legend map[string]*brush, k, transparent, erase string, opaque bool) (cell, *errs.Error) {
	if b, ok := legend[k]; ok {
		if b.c.InPalette {
			return cell{index: b.c.Index}, nil
		}
		return cell{brush: b}, nil
	}
	switch k {
	case transparent:
		return cell{skip: !opaque, index: doc.Transparent}, nil
	case erase:
		return cell{index: doc.Transparent}, nil
	}
	if i, ok := d.Palette.ByKey(k); ok {
		return cell{index: i}, nil
	}
	if !doc.ValidKey(k, d.KeyWidth) {
		e := errs.New(errs.InvalidColor, "%q can't be a palette key", k)
		if strings.Contains(k, "+") {
			e = e.WithHint("+ marks a blended pixel in show --composite; look its color up with pxo get")
		} else {
			e = e.WithHint("a cell is a palette key, a --legend letter, %s to leave the pixel alone or %s to erase it", transparent, erase)
		}
		return cell{}, e
	}
	keys := make([]string, 0, d.Palette.Len()+len(legend))
	for _, e := range d.Palette.Entries[1:] {
		keys = append(keys, e.Key)
	}
	keys = append(keys, slices.Sorted(maps.Keys(legend))...)
	return cell{}, errs.New(errs.UnknownColor, "unknown key %q", k).
		WithHint("add %q to --legend or the palette; the keys are: %s", k, strings.Join(keys, " "))
}

// parseLegend reads `letter=color` pairs separated by spaces. Every color
// is resolved now, so a bad one fails even when no cell uses it. Letters
// whose colors resolve to the same raw hex color share one brush, so the
// color gets one auto entry.
func parseLegend(ctx *Ctx, text string) (map[string]*brush, error) {
	out := map[string]*brush{}
	hex := map[doc.RGBA]*brush{}
	kw := ctx.Doc.KeyWidth
	for pair := range strings.FieldsSeq(text) {
		k, v, ok := strings.Cut(pair, "=")
		switch {
		case !ok || v == "":
			return nil, errs.New(errs.Usage, "bad legend entry %q", pair).WithArg("legend").
				WithHint(`the legend is letter=color pairs separated by spaces, such as "s=skin S=skin_dark"`)
		case !doc.ValidKey(k, kw):
			return nil, errs.New(errs.Usage, "bad legend letter %q", k).WithArg("legend").
				WithHint("a legend letter is as wide as a palette key (%d) and can't be %s, %s or another reserved character", kw, doc.TransparentKey(kw), doc.EraseKey(kw))
		case out[k] != nil:
			return nil, errs.New(errs.Usage, "legend letter %q is given twice", k).WithArg("legend")
		}
		b, err := newBrush(ctx, "legend", v)
		if err != nil {
			return nil, err
		}
		if !b.c.InPalette {
			if shared, ok := hex[b.c.Color]; ok {
				b = shared
			} else {
				hex[b.c.Color] = b
			}
		}
		out[k] = b
	}
	return out, nil
}

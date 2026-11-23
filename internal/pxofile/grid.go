package pxofile

import (
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// rleMinWidth: writers always use RLE for grids wider than this.
const rleMinWidth = 128

// pixGrid is a decoded rectangle of palette indices.
type pixGrid struct {
	W, H int
	Pix  []doc.Index
}

// decoder maps keys to palette indices for one document.
type decoder struct {
	kw   int
	keys map[string]doc.Index
}

func newDecoder(d *doc.Doc) *decoder {
	keys := make(map[string]doc.Index, d.Palette.Len())
	for i, e := range d.Palette.Entries {
		keys[e.Key] = doc.Index(i)
	}
	return &decoder{kw: d.KeyWidth, keys: keys}
}

func (dc *decoder) key(where, k string) (doc.Index, error) {
	if i, ok := dc.keys[k]; ok {
		return i, nil
	}
	if k == doc.EraseKey(dc.kw) {
		return 0, invalid("%s: the erase marker %q is never stored in a grid", where, k).
			WithHint("use %q for transparent pixels", doc.TransparentKey(dc.kw))
	}
	return 0, invalid("%s: unknown palette key %q", where, k)
}

func mismatch(format string, a ...any) *errs.Error { return errs.New(errs.GridMismatch, format, a...) }

// decodeGrid decodes a letter grid: one row per line, key_width characters
// per pixel. The first newline after ”' is already gone (TOML strips it), a
// single trailing newline is ignored, and CR before LF is tolerated.
func (dc *decoder) decodeGrid(where, s string) (pixGrid, error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return pixGrid{}, mismatch("%s: grid is empty", where)
	}
	rows := strings.Split(s, "\n")
	w := len(rows[0])
	if w%dc.kw != 0 {
		return pixGrid{}, mismatch("%s: row 0 is %d characters, not a multiple of key_width %d", where, w, dc.kw)
	}
	g := pixGrid{W: w / dc.kw, H: len(rows), Pix: make([]doc.Index, 0, w/dc.kw*len(rows))}
	for y, row := range rows {
		if len(row) != w {
			return pixGrid{}, mismatch("%s: row %d is %d characters, row 0 is %d", where, y, len(row), w).
				WithHint("every row of a grid must have the same width")
		}
		for x := 0; x < w; x += dc.kw {
			i, err := dc.key(where+" row "+strconv.Itoa(y), row[x:x+dc.kw])
			if err != nil {
				return pixGrid{}, err
			}
			g.Pix = append(g.Pix, i)
		}
	}
	return g, nil
}

// decodeRLE decodes RLE rows. Each row is space-separated <count><key>
// tokens. The key is the last key_width characters of a token and the count
// is the rest, so digit keys are unambiguous ("121" is 12 x "1").
func (dc *decoder) decodeRLE(where string, rows []string) (pixGrid, error) {
	if len(rows) == 0 {
		return pixGrid{}, mismatch("%s: rle rows are empty", where)
	}
	g := pixGrid{H: len(rows)}
	for y, row := range rows {
		rw := where + " row " + strconv.Itoa(y)
		n := 0
		for _, tok := range strings.Fields(row) {
			if len(tok) <= dc.kw {
				return pixGrid{}, invalid("%s: rle token %q needs a count and a key", rw, tok)
			}
			cs := tok[:len(tok)-dc.kw]
			count, err := strconv.Atoi(cs)
			if err != nil || count < 1 || cs[0] < '0' || cs[0] > '9' {
				return pixGrid{}, invalid("%s: rle token %q has an invalid count", rw, tok)
			}
			if n+count > doc.MaxSize {
				return pixGrid{}, mismatch("%s: row is wider than %d", rw, doc.MaxSize)
			}
			i, err := dc.key(rw, tok[len(tok)-dc.kw:])
			if err != nil {
				return pixGrid{}, err
			}
			for range count {
				g.Pix = append(g.Pix, i)
			}
			n += count
		}
		if y == 0 {
			g.W = n
		}
		if n != g.W || n == 0 {
			return pixGrid{}, mismatch("%s: row %d is %d pixels wide, row 0 is %d", where, y, n, g.W)
		}
	}
	return g, nil
}

// encoded is a grid ready to write: either grid text or RLE rows.
type encoded struct {
	Grid string   // set when RLE is false
	RLE  bool     //
	Rows []string // set when RLE is true
}

// encode writes a w x h rectangle of pix (row-major, stride w). It uses RLE
// when the grid is wider than 128 px or when the RLE rows (token text only)
// are less than half the size of the raw grid.
func encode(pix []doc.Index, w, h int, keys []string) encoded {
	kw := len(keys[0])
	rows := make([]string, h)
	rleLen := 0
	var b strings.Builder
	for y := range h {
		b.Reset()
		line := pix[y*w : (y+1)*w]
		for x := 0; x < w; {
			run := 1
			for x+run < w && line[x+run] == line[x] {
				run++
			}
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(run))
			b.WriteString(keys[line[x]])
			x += run
		}
		rows[y] = b.String()
		rleLen += len(rows[y])
	}
	if w > rleMinWidth || 2*rleLen < w*h*kw {
		return encoded{RLE: true, Rows: rows}
	}
	b.Reset()
	b.Grow((w*kw + 1) * h)
	for y := range h {
		for _, p := range pix[y*w : (y+1)*w] {
			b.WriteString(keys[p])
		}
		b.WriteByte('\n')
	}
	return encoded{Grid: b.String()}
}

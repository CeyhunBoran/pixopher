package export

import (
	"image"
	"image/color"
	"image/png"
	"io"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

const (
	// MaxScale is the largest --scale (docs/export-import.md).
	MaxScale = 64
	// MaxSide is the largest width or height of an exported image (D58).
	MaxSide = 8192
	// MaxIndexed is the most distinct colors an indexed PNG can hold.
	MaxIndexed = 256
)

// PNGOptions controls PNG output.
type PNGOptions struct {
	// Scale is the integer nearest-neighbor upscale, 1 to MaxScale. Zero
	// means 1.
	Scale int
	// RGBA writes truecolor even when the image has few enough colors for
	// an indexed PNG. The encoder still writes RGB, without alpha, when
	// every pixel is opaque.
	RGBA bool
	// Palette is the preferred order of an indexed PNG's palette, usually
	// the document's palette: the colors of the image found in it come
	// first, in its order, then the others in the order they first appear.
	Palette []doc.RGBA
}

// PNG encodes img, which must start at 0,0 with a stride of 4*width (as
// image.NewNRGBA makes it), to w. With at most MaxIndexed
// distinct colors (after mapping every fully transparent pixel to 0,0,0,0)
// and without opt.RGBA, it writes an indexed PNG with a PLTE chunk and,
// when needed, tRNS; otherwise truecolor. The bytes depend only on img and
// opt (D58).
func PNG(w io.Writer, img *image.NRGBA, opt PNGOptions) error {
	scale := max(opt.Scale, 1)
	var out image.Image
	if !opt.RGBA {
		if pal, idx, ok := indexed(img, opt.Palette); ok {
			out = scalePaletted(img.Rect.Dx(), img.Rect.Dy(), pal, idx, scale)
		}
	}
	if out == nil {
		out = scaleNRGBA(img, scale)
	}
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	return enc.Encode(w, out)
}

// key packs a non-premultiplied color, with alpha 0 mapped to 0.
func key(p []byte) uint32 {
	if p[3] == 0 {
		return 0
	}
	return uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
}

func unkey(k uint32) color.NRGBA {
	return color.NRGBA{R: uint8(k >> 24), G: uint8(k >> 16), B: uint8(k >> 8), A: uint8(k)}
}

// indexed returns the palette and per-pixel indices of img, or false when
// it has more than MaxIndexed colors.
func indexed(img *image.NRGBA, prefer []doc.RGBA) (color.Palette, []uint8, bool) {
	used := map[uint32]bool{}
	var order []uint32 // first appearance
	for i := 0; i < len(img.Pix); i += 4 {
		k := key(img.Pix[i : i+4])
		if !used[k] {
			if len(order) == MaxIndexed {
				return nil, nil, false
			}
			used[k] = true
			order = append(order, k)
		}
	}
	index := make(map[uint32]uint8, len(order))
	var pal color.Palette
	add := func(k uint32) {
		if _, dup := index[k]; dup || !used[k] {
			return
		}
		index[k] = uint8(len(pal))
		pal = append(pal, unkey(k))
	}
	for _, c := range prefer {
		add(key([]byte{c.R, c.G, c.B, c.A}))
	}
	for _, k := range order {
		add(k)
	}
	idx := make([]uint8, len(img.Pix)/4)
	for i := range idx {
		idx[i] = index[key(img.Pix[i*4:i*4+4])]
	}
	return pal, idx, true
}

func scalePaletted(w, h int, pal color.Palette, idx []uint8, s int) *image.Paletted {
	out := image.NewPaletted(image.Rect(0, 0, w*s, h*s), pal)
	for y := range h {
		row := out.Pix[y*s*out.Stride : y*s*out.Stride+w*s]
		for x := range w {
			v := idx[y*w+x]
			for i := range s {
				row[x*s+i] = v
			}
		}
		for i := 1; i < s; i++ {
			copy(out.Pix[(y*s+i)*out.Stride:], row)
		}
	}
	return out
}

var clear4 [4]byte

func scaleNRGBA(img *image.NRGBA, s int) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w*s, h*s))
	for y := range h {
		row := out.Pix[y*s*out.Stride : y*s*out.Stride+w*s*4]
		for x := range w {
			o := y*img.Stride + x*4
			p := img.Pix[o : o+4 : o+4]
			if p[3] == 0 {
				p = clear4[:]
			}
			for i := range s {
				copy(row[(x*s+i)*4:], p)
			}
		}
		for i := 1; i < s; i++ {
			copy(out.Pix[(y*s+i)*out.Stride:], row)
		}
	}
	return out
}

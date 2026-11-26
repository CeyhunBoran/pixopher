package export

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/golden"
)

// sprite is a 4x3 image: a transparent border column, two opaque colors and
// a half-transparent one.
func sprite() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	ink := color.NRGBA{R: 0x1a, G: 0x1c, B: 0x2c, A: 255}
	red := color.NRGBA{R: 0xb1, G: 0x3e, B: 0x53, A: 255}
	haze := color.NRGBA{R: 255, G: 255, B: 255, A: 128}
	for y := range 3 {
		img.SetNRGBA(1, y, ink)
		img.SetNRGBA(2, y, red)
		img.SetNRGBA(3, y, ink)
	}
	img.SetNRGBA(3, 1, haze)
	return img
}

func encode(t *testing.T, img *image.NRGBA, opt PNGOptions) ([]byte, image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := PNG(&buf, img, opt); err != nil {
		t.Fatal(err)
	}
	out, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), out
}

// sameScaled checks that got is src upscaled by s, pixel by pixel.
func sameScaled(t *testing.T, got image.Image, src *image.NRGBA, s int) {
	t.Helper()
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if b := got.Bounds(); b != image.Rect(0, 0, w*s, h*s) {
		t.Fatalf("bounds %v, want %dx%d", b, w*s, h*s)
	}
	for y := range h * s {
		for x := range w * s {
			want := src.NRGBAAt(x/s, y/s)
			if want.A == 0 {
				want = color.NRGBA{}
			}
			if c := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA); c != want {
				t.Fatalf("pixel %d,%d is %v, want %v", x, y, c, want)
			}
		}
	}
}

func TestPNGIndexed(t *testing.T) {
	src := sprite()
	prefer := []doc.RGBA{{}, {R: 0xb1, G: 0x3e, B: 0x53, A: 255}, {R: 1, A: 255}, {R: 0x1a, G: 0x1c, B: 0x2c, A: 255}}
	data, out := encode(t, src, PNGOptions{Scale: 3, Palette: prefer})
	p, ok := out.(*image.Paletted)
	if !ok {
		t.Fatalf("decoded %T, want an indexed PNG", out)
	}
	// Used palette colors in palette order (the unused #010000 is left
	// out), then the others in order of appearance.
	want := color.Palette{
		color.NRGBA{}, color.NRGBA{R: 0xb1, G: 0x3e, B: 0x53, A: 255},
		color.NRGBA{R: 0x1a, G: 0x1c, B: 0x2c, A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 128},
	}
	if len(p.Palette) != len(want) {
		t.Fatalf("palette %v, want %v", p.Palette, want)
	}
	for i := range want {
		if color.NRGBAModel.Convert(p.Palette[i]) != want[i] {
			t.Fatalf("palette %v, want %v", p.Palette, want)
		}
	}
	sameScaled(t, out, src, 3)
	golden.Check(t, filepath.Join("testdata", "indexed.png"), data)
}

func TestPNGRGBAOverIndexLimit(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 17, 16))
	for i := range 17 * 16 {
		src.SetNRGBA(i%17, i/17, color.NRGBA{R: uint8(i), G: uint8(i >> 8), A: 255})
	}
	data, out := encode(t, src, PNGOptions{Scale: 2})
	if _, ok := out.(*image.Paletted); ok {
		t.Fatal("272 colors made an indexed PNG")
	}
	sameScaled(t, out, src, 2)
	golden.Check(t, filepath.Join("testdata", "rgba.png"), data)

	// Exactly 256 colors still fit.
	src = image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := range 256 {
		src.SetNRGBA(i%16, i/16, color.NRGBA{R: uint8(i), A: 255})
	}
	if _, out = encode(t, src, PNGOptions{}); !isPaletted(out) {
		t.Fatal("256 colors didn't make an indexed PNG")
	}
}

func isPaletted(img image.Image) bool { _, ok := img.(*image.Paletted); return ok }

func TestPNGForcedRGBA(t *testing.T) {
	src := sprite()
	_, out := encode(t, src, PNGOptions{RGBA: true, Scale: 2})
	if _, ok := out.(*image.Paletted); ok {
		t.Fatal("--rgba made an indexed PNG")
	}
	sameScaled(t, out, src, 2)
}

func TestPNGTransparentAndDuplicateColorsMerge(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 9, G: 9, B: 9}) // alpha 0, not black
	src.SetNRGBA(2, 0, color.NRGBA{R: 7, A: 255})
	// The preferred order repeats a color and holds other alpha-0 colors.
	prefer := []doc.RGBA{{R: 5}, {R: 7, A: 255}, {R: 7, A: 255}, {}}
	_, out := encode(t, src, PNGOptions{Palette: prefer})
	p := out.(*image.Paletted)
	want := color.Palette{color.NRGBA{}, color.NRGBA{R: 7, A: 255}}
	if len(p.Palette) != 2 || color.NRGBAModel.Convert(p.Palette[0]) != want[0] ||
		color.NRGBAModel.Convert(p.Palette[1]) != want[1] {
		t.Fatalf("palette %v, want %v", p.Palette, want)
	}
	if !bytes.Equal(p.Pix, []uint8{0, 0, 1}) {
		t.Fatalf("indices %v", p.Pix)
	}
	// Truecolor writes every transparent pixel as 0,0,0,0 too.
	_, out = encode(t, src, PNGOptions{RGBA: true})
	if c := out.(*image.NRGBA).NRGBAAt(0, 0); c != (color.NRGBA{}) {
		t.Fatalf("transparent pixel %v, want 0,0,0,0", c)
	}
}

func TestPNGIsDeterministic(t *testing.T) {
	a, _ := encode(t, sprite(), PNGOptions{Scale: 5})
	b, _ := encode(t, sprite(), PNGOptions{Scale: 5})
	if !bytes.Equal(a, b) {
		t.Fatal("two encodes of the same image differ")
	}
}

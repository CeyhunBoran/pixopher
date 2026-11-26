package render

import (
	"image/color"
	"slices"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// testDoc is a 2x1 document with entries a (opaque red), b (opaque blue),
// h (half-transparent white) and z (alpha 0 but not black).
func testDoc(t *testing.T) *doc.Doc {
	t.Helper()
	d, err := doc.New(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	d.Palette.Entries = append(d.Palette.Entries,
		doc.Entry{Key: "a", Name: "red", Color: doc.RGBA{R: 255, A: 255}},
		doc.Entry{Key: "b", Name: "blue", Color: doc.RGBA{B: 255, A: 255}},
		doc.Entry{Key: "h", Name: "haze", Color: doc.RGBA{R: 255, G: 255, B: 255, A: 128}},
		doc.Entry{Key: "z", Name: "ghost", Color: doc.RGBA{R: 9, G: 9, B: 9}},
	)
	return d
}

func at(t *testing.T, d *doc.Doc, x int) color.NRGBA {
	t.Helper()
	img, _ := Composite(d, 0)
	return img.NRGBAAt(x, 0)
}

func TestCompositeSingleLayerKeepsPaletteColors(t *testing.T) {
	d := testDoc(t)
	d.Cels[doc.CelKey{}] = &doc.Cel{Pix: []doc.Index{1, 3}}
	img, warnings := Composite(d, 0)
	if len(warnings) != 0 {
		t.Fatalf("warnings %q", warnings)
	}
	want := []byte{255, 0, 0, 255, 255, 255, 255, 128}
	if !slices.Equal(img.Pix, want) {
		t.Fatalf("pix %v, want %v", img.Pix, want)
	}
}

func TestCompositeTransparentIsZero(t *testing.T) {
	d := testDoc(t)
	d.Cels[doc.CelKey{}] = &doc.Cel{Pix: []doc.Index{4, 0}}
	img, _ := Composite(d, 0)
	if !slices.Equal(img.Pix, make([]byte, 8)) {
		t.Fatalf("pix %v, want all zero", img.Pix)
	}
}

func TestCompositeStacksVisibleLayers(t *testing.T) {
	d := testDoc(t)
	d.Layers = append(d.Layers, doc.NewLayer("top"), doc.NewLayer("hidden"))
	d.Layers[2].Visible = false
	d.Cels[doc.CelKey{Layer: 0}] = &doc.Cel{Pix: []doc.Index{1, 1}}
	d.Cels[doc.CelKey{Layer: 1}] = &doc.Cel{Pix: []doc.Index{2, 3}}
	d.Cels[doc.CelKey{Layer: 2}] = &doc.Cel{Pix: []doc.Index{2, 2}}
	if got, want := at(t, d, 0), (color.NRGBA{B: 255, A: 255}); got != want {
		t.Errorf("opaque over opaque: %v, want %v", got, want)
	}
	// White at alpha 128 over opaque red: 255*128/255 + 255*127/255.
	if got, want := at(t, d, 1), (color.NRGBA{R: 255, G: 128, B: 128, A: 255}); got != want {
		t.Errorf("half white over red: %v, want %v", got, want)
	}
}

func TestCompositeLayerOpacity(t *testing.T) {
	d := testDoc(t)
	d.Layers = append(d.Layers, doc.NewLayer("top"))
	d.Layers[1].Opacity = 51 // 20%
	d.Cels[doc.CelKey{Layer: 1}] = &doc.Cel{Pix: []doc.Index{2, 3}}
	if got, want := at(t, d, 0), (color.NRGBA{B: 255, A: 51}); got != want {
		t.Errorf("blue at 20%%: %v, want %v", got, want)
	}
	// 128 * 51 / 255 = 25.6, rounded to 26.
	if got, want := at(t, d, 1), (color.NRGBA{R: 255, G: 255, B: 255, A: 26}); got != want {
		t.Errorf("haze at 20%%: %v, want %v", got, want)
	}
	d.Cels[doc.CelKey{Layer: 0}] = &doc.Cel{Pix: []doc.Index{1, 1}}
	// Blue at alpha 51 over opaque red: red 255*204/255, blue 255*51/255.
	if got, want := at(t, d, 0), (color.NRGBA{R: 204, B: 51, A: 255}); got != want {
		t.Errorf("blue at 20%% over red: %v, want %v", got, want)
	}
	d.Layers[1].Opacity = 0
	if got, want := at(t, d, 0), (color.NRGBA{R: 255, A: 255}); got != want {
		t.Errorf("opacity 0: %v, want %v", got, want)
	}
}

func TestCompositeOverTranslucent(t *testing.T) {
	d := testDoc(t)
	d.Layers = append(d.Layers, doc.NewLayer("top"))
	d.Cels[doc.CelKey{Layer: 0}] = &doc.Cel{Pix: []doc.Index{3, 0}}
	d.Cels[doc.CelKey{Layer: 1}] = &doc.Cel{Pix: []doc.Index{3, 0}}
	// Two layers of alpha 128: a = 128 + 128*127/255 = 191.75, rounded to 192.
	if got, want := at(t, d, 0), (color.NRGBA{R: 255, G: 255, B: 255, A: 192}); got != want {
		t.Errorf("haze over haze: %v, want %v", got, want)
	}
}

func TestCompositeFollowsLinksAndFrames(t *testing.T) {
	d := testDoc(t)
	d.Frames = append(d.Frames, doc.Frame{DurationMS: 100})
	d.Cels[doc.CelKey{Frame: 0}] = &doc.Cel{Pix: []doc.Index{1, 0}}
	d.Cels[doc.CelKey{Frame: 1}] = &doc.Cel{Linked: true, LinkFrame: 0}
	img, _ := Composite(d, 1)
	if got, want := img.NRGBAAt(0, 0), (color.NRGBA{R: 255, A: 255}); got != want {
		t.Errorf("linked cel: %v, want %v", got, want)
	}
}

func TestCompositeWarnsOnOtherBlendModes(t *testing.T) {
	d := testDoc(t)
	d.Layers[0].Blend = "multiply"
	d.Cels[doc.CelKey{}] = &doc.Cel{Pix: []doc.Index{1, 0}}
	img, warnings := Composite(d, 0)
	want := []string{`layer "main": blend mode "multiply" is not supported yet; drawn as normal`}
	if !slices.Equal(warnings, want) {
		t.Fatalf("warnings %q, want %q", warnings, want)
	}
	if got := img.NRGBAAt(0, 0); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("pixel %v, want red", got)
	}
	// An empty layer draws nothing, so it doesn't warn.
	d.Cels = map[doc.CelKey]*doc.Cel{}
	if _, warnings := Composite(d, 0); len(warnings) != 0 {
		t.Fatalf("empty layer warned: %q", warnings)
	}
}

// Package doc is the document model: Doc, Palette, Ramp, Layer, Frame, Cel, Region,
// Anchor and Part.
//
// Pixels are stored as palette indices. Index 0 is always the transparent
// entry ".", so the zero value of a pixel buffer is an empty canvas. Keys are
// a file-format concern: they only matter when a document is parsed or
// serialized, which is why changing key_width never touches pixel data.
package doc

import (
	"time"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// Format and size limits.
const (
	FormatVersion = 1    // the `pixopher = N` header this build reads and writes
	MaxSize       = 1024 // maximum canvas width and height (D9)

	DefaultLayerName  = "main" // D17
	DefaultDurationMS = 100
	DefaultOpacity    = 255
	DefaultBlend      = "normal"
	DefaultDirection  = "forward"
)

// Symmetry modes.
const (
	SymOff = "off"
	SymX   = "x"
	SymY   = "y"
	SymXY  = "xy"
)

// Tag directions.
const (
	DirForward  = "forward"
	DirReverse  = "reverse"
	DirPingpong = "pingpong"
)

// RGBA is a non-premultiplied 8-bit color.
type RGBA struct{ R, G, B, A uint8 }

// Index is a position in the palette. Pixels store indices.
type Index uint16

// Transparent is the index of the reserved "." entry.
const Transparent Index = 0

// Point is a pixel coordinate.
type Point struct{ X, Y int }

// Rect is a rectangle given by its top-left corner and size, as in the file
// format's [x, y, w, h].
type Rect struct{ X, Y, W, H int }

// In reports whether r lies entirely inside a w x h canvas.
func (r Rect) In(w, h int) bool {
	return r.W >= 1 && r.H >= 1 && r.X >= 0 && r.Y >= 0 && r.X+r.W <= w && r.Y+r.H <= h
}

// Doc is one pixopher document.
type Doc struct {
	Version  int // format version; FormatVersion for documents this build writes
	W, H     int
	Rev      int
	KeyWidth int // characters per pixel in grids: 1 or 2

	Meta     Meta
	Palette  Palette
	Ramps    map[string]Ramp
	Symmetry Symmetry
	Layers   []Layer
	Frames   []Frame
	Tags     []Tag
	Regions  map[string]Rect
	Anchors  map[string]Point // default positions; frames may override
	Variants map[string]Variant
	Lint     LintConfig
	Parts    map[string]Part
	Cels     map[CelKey]*Cel
}

// Meta is free-form document metadata.
type Meta struct {
	Title   string
	Created time.Time // zero when unset
}

// Ramp is an ordered list of palette entries, dark to light.
type Ramp struct {
	Steps []Index
	Base  int // step used for the bare ramp name and for relative steps
}

// DefaultBase is the base step of an n-step ramp when none is given: the
// middle step, rounding toward the dark end for even lengths.
func DefaultBase(n int) int { return (n - 1) / 2 }

// Symmetry is the document's drawing symmetry.
type Symmetry struct {
	Mode  string  // SymOff, SymX, SymY or SymXY
	AxisX float64 // vertical mirror axis; multiples of 0.5
	AxisY float64 // horizontal mirror axis; multiples of 0.5
}

// DefaultSymmetry is symmetry off with both axes on the canvas center.
func DefaultSymmetry(w, h int) Symmetry {
	return Symmetry{Mode: SymOff, AxisX: float64(w) / 2, AxisY: float64(h) / 2}
}

// Layer is one layer. Its index in Doc.Layers is its stacking order, bottom
// first.
type Layer struct {
	Name    string
	Visible bool
	Locked  bool
	Opacity uint8
	Blend   string
}

// NewLayer returns a visible, unlocked, opaque layer with normal blending.
func NewLayer(name string) Layer {
	return Layer{Name: name, Visible: true, Opacity: DefaultOpacity, Blend: DefaultBlend}
}

// Frame is one animation frame.
type Frame struct {
	DurationMS int
	Anchors    map[string]Point // per-frame anchor overrides; nil when none
}

// Tag is a named range of frames (0-based, inclusive).
type Tag struct {
	Name      string
	From, To  int
	Direction string
}

// Variant is a named set of palette overrides.
type Variant struct {
	Ramps   map[string][]RGBA // ramp name to new colors, one per step
	Entries map[string]RGBA   // palette entry name to new color
}

// LintConfig is the [lint] section.
type LintConfig struct {
	MaxColors int      // 0 means no limit
	Disable   []string // rule names, sorted
}

// Part is a reusable stamp with its own grid, using the document palette.
type Part struct {
	W, H int
	Pix  []Index // row-major, len W*H
}

// CelKey identifies a cel by layer and frame index.
type CelKey struct{ Layer, Frame int }

// Cel holds the pixels of one (layer, frame). In memory a cel always covers
// the whole canvas; writers crop it to its content.
type Cel struct {
	Pix []Index // row-major, len Doc.W*Doc.H; nil when Linked

	// Linked means "same pixels as this layer's cel in frame LinkFrame".
	Linked    bool
	LinkFrame int
}

// New returns an empty w x h document: one layer named "main", one frame of
// 100 ms, a palette holding only ".", key width 1 and rev 0. Meta.Created is
// left unset; callers that create files set it from their own clock.
func New(w, h int) (*Doc, error) {
	if err := checkSize(w, h); err != nil {
		return nil, err
	}
	return &Doc{
		Version:  FormatVersion,
		W:        w,
		H:        h,
		KeyWidth: 1,
		Palette:  NewPalette(1),
		Ramps:    map[string]Ramp{},
		Symmetry: DefaultSymmetry(w, h),
		Layers:   []Layer{NewLayer(DefaultLayerName)},
		Frames:   []Frame{{DurationMS: DefaultDurationMS}},
		Regions:  map[string]Rect{},
		Anchors:  map[string]Point{},
		Variants: map[string]Variant{},
		Parts:    map[string]Part{},
		Cels:     map[CelKey]*Cel{},
	}, nil
}

func checkSize(w, h int) error {
	if w > MaxSize || h > MaxSize {
		return errs.New(errs.TooLarge, "size %dx%d is too large", w, h).
			WithHint("the maximum canvas is %dx%d", MaxSize, MaxSize).WithArg("size")
	}
	if w < 1 || h < 1 {
		return errs.New(errs.OutOfBounds, "size %dx%d is empty", w, h).
			WithHint("width and height must be 1..%d", MaxSize).WithArg("size")
	}
	return nil
}

// LayerIndex returns the index of the layer called name, or -1.
func (d *Doc) LayerIndex(name string) int {
	for i, l := range d.Layers {
		if l.Name == name {
			return i
		}
	}
	return -1
}

// Pixels returns the pixels of (layer, frame), following a link, or nil if
// the cel is empty.
func (d *Doc) Pixels(layer, frame int) []Index {
	c := d.Cels[CelKey{layer, frame}]
	if c != nil && c.Linked {
		c = d.Cels[CelKey{layer, c.LinkFrame}]
	}
	if c == nil || c.Linked {
		return nil
	}
	return c.Pix
}

// Empty reports whether the cel has no visible content of its own. A linked
// cel is never empty by itself; see Doc.PruneEmptyCels.
func (c *Cel) Empty() bool {
	if c.Linked {
		return false
	}
	for _, p := range c.Pix {
		if p != Transparent {
			return false
		}
	}
	return true
}

// PruneEmptyCels removes cels without content, and links that point to a
// removed cel. A missing cel already means "empty", so this never changes
// what the document looks like.
func (d *Doc) PruneEmptyCels() {
	for k, c := range d.Cels {
		if !c.Linked && c.Empty() {
			delete(d.Cels, k)
		}
	}
	for k, c := range d.Cels {
		if c.Linked {
			if t := d.Cels[CelKey{k.Layer, c.LinkFrame}]; t == nil || t.Linked {
				delete(d.Cels, k)
			}
		}
	}
}

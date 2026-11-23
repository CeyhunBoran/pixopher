package ops

import (
	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Op is one registered operation.
type Op interface {
	Spec() Spec
	// Apply changes ctx.Doc. Pixel writes go through ctx.Set so they are
	// recorded; header fields (palette, ramps, ...) may be changed directly,
	// and Ctx.Finish detects that. On error the engine discards the document.
	Apply(ctx *Ctx, args Args) error
}

// Args holds parsed argument values keyed by ArgSpec.Name: string for
// KindString, KindEnum, KindColor and KindGrid, int for KindInt, bool for
// KindBool, doc.Point for KindPoint, doc.Rect for KindRect, and Size for
// KindSize. A repeated positional holds a slice of its element type.
// Missing arguments are absent; the getters then return the zero value.
type Args map[string]any

// Size is a width and height.
type Size struct{ W, H int }

// String returns the string argument called name.
func (a Args) String(name string) string { v, _ := a[name].(string); return v }

// Int returns the int argument called name.
func (a Args) Int(name string) int { v, _ := a[name].(int); return v }

// Bool returns the bool argument called name.
func (a Args) Bool(name string) bool { v, _ := a[name].(bool); return v }

// Point returns the point argument called name.
func (a Args) Point(name string) doc.Point { v, _ := a[name].(doc.Point); return v }

// Rect returns the rect argument called name.
func (a Args) Rect(name string) doc.Rect { v, _ := a[name].(doc.Rect); return v }

// Size returns the size argument called name.
func (a Args) Size(name string) Size { v, _ := a[name].(Size); return v }

// Has reports whether the argument called name was given.
func (a Args) Has(name string) bool { _, ok := a[name]; return ok }

// Call is one op invocation.
type Call struct {
	Op   Op
	Args Args
	// Line is the normalized script line, such as "rect 6,1 4,3 ink". It is
	// stored in the history log for replay and captions.
	Line string
}

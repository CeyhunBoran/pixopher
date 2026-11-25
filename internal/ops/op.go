package ops

import (
	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Op is one registered operation. Every op is either a Mutator (its spec
// has Mutates) or an Observer (D38, D52); Registry.Register checks this.
type Op interface {
	Spec() Spec
}

// Mutator is an op that changes a document.
type Mutator interface {
	Op
	// Apply changes ctx.Doc. Pixel writes go through ctx.Set so they are
	// recorded; header fields (palette, ramps, ...) may be changed directly,
	// and Ctx.Finish detects that. On error the engine discards the document.
	Apply(ctx *Ctx, args Args) error
}

// Creator is implemented by ops whose spec has Creates (D39), such as new.
// Create returns the new, empty document for args. The engine then runs
// Apply on it as the creating step (new paints --bg there) and writes the
// result at rev 0, without a history entry.
type Creator interface {
	Mutator
	Create(args Args) (*doc.Doc, error)
}

// Observer is an op that only looks at a document (D38): info, palette
// list, show, get and stats, and later lint. It gets the loaded document
// and never a Ctx, so it can't write, bump the rev or log by construction.
// The engine takes no lock for it. Observe must not modify d.
type Observer interface {
	Op
	Observe(d *doc.Doc, args Args) (Report, error)
}

// Report is what an Observer returns. Its JSON form, from encoding/json
// on the value, is the --json output after "ok": true, so it must marshal
// to an object and its field names are public API (docs/cli.md). Text is
// the human form, ending in a newline.
type Report interface {
	Text() string
}

// Args holds parsed argument values keyed by ArgSpec.Name: string for
// KindString, KindEnum, KindColor and KindGrid, int for KindInt, bool for
// KindBool, doc.Point for KindPoint, doc.Rect for KindRect and KindArea,
// and Size for KindSize. A repeated positional holds a slice of its element
// type.
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

// Call is one invocation of a mutating op.
type Call struct {
	Op   Mutator
	Args Args
	// Line is the canonical script line from Format, such as
	// "rect 6,1 4,3 ink". It is stored in the history log for replay and
	// captions.
	Line string
}

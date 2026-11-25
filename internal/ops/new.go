package ops

import (
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// Ext is the document file extension, matched case-sensitively.
const Ext = ".pxo"

type newOp struct{}

func (newOp) Spec() Spec {
	return Spec{
		Name: "new", Group: "document", Summary: "Create a document", Mutates: true, Creates: true,
		Args: []ArgSpec{
			{Name: "file", Kind: KindString, Required: true, Help: "path of the new document, ending in .pxo"},
			{Name: "size", Kind: KindSize, Flag: true, Default: "32x32", Help: "canvas size, up to 1024x1024"},
			{Name: "bg", Kind: KindColor, Flag: true, Help: "fill the canvas with this color (a #hex color adds an entry)"},
		},
		Examples: []string{"new hero.pxo", "new icon.pxo --size 16x16 --bg '#1a1c2c'"},
	}
}

// Create returns the empty document: one layer, one frame and a palette
// holding only ".". Built-in palettes come with --palette (D46).
func (newOp) Create(args Args) (*doc.Doc, error) {
	if f := args.String("file"); !strings.HasSuffix(f, Ext) || f == Ext {
		return nil, errs.New(errs.Usage, "document %q must end in %s", f, Ext).
			WithHint("use a name like hero%s", Ext).WithArg("file")
	}
	s := args.Size("size")
	return doc.New(s.W, s.H)
}

// Apply paints the background, if one is given.
func (newOp) Apply(ctx *Ctx, args Args) error {
	if !args.Has("bg") {
		return nil
	}
	b, err := newBrush(ctx, "bg", args.String("bg"))
	if err != nil {
		return err
	}
	for y := range ctx.Doc.H {
		for x := range ctx.Doc.W {
			b.set(x, y)
		}
	}
	return b.err
}

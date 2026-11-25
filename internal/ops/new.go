package ops

import (
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/palettes"
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
			{Name: "palette", Kind: KindString, Flag: true, Default: palettes.Default, Help: "built-in palette to start with (" + paletteChoices() + " for an empty palette)"},
		},
		Examples: []string{"new hero.pxo", "new icon.pxo --size 16x16 --bg ink", "new blank.pxo --palette none --bg '#1a1c2c'"},
	}
}

// PaletteNone is the --palette value for a palette holding only ".".
const PaletteNone = "none"

// paletteChoices lists the --palette values: the built-in names, then none
// ("default or none", "a, b or none").
func paletteChoices() string {
	names := append(palettes.Names(), PaletteNone)
	return strings.Join(names[:len(names)-1], ", ") + " or " + PaletteNone
}

// Create returns the empty document: one layer, one frame, and the palette
// and ramps of the built-in palette named by --palette, or only "." for
// none (D46, D53).
func (newOp) Create(args Args) (*doc.Doc, error) {
	if f := args.String("file"); !strings.HasSuffix(f, Ext) || f == Ext {
		return nil, errs.New(errs.Usage, "document %q must end in %s", f, Ext).
			WithHint("use a name like hero%s", Ext).WithArg("file")
	}
	var pal *doc.Doc
	if name := args.String("palette"); name != PaletteNone {
		var ok bool
		if pal, ok = palettes.Load(name); !ok {
			return nil, errs.New(errs.Usage, "no built-in palette %q", name).
				WithHint("use %s", paletteChoices()).WithArg("palette")
		}
	}
	s := args.Size("size")
	d, err := doc.New(s.W, s.H)
	if err != nil || pal == nil {
		return d, err
	}
	d.Palette, d.Ramps = pal.Palette, pal.Ramps
	return d, nil
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

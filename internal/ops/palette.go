package ops

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

func init() {
	for _, op := range []Op{
		paletteListOp{}, paletteAddOp{}, paletteSetOp{}, paletteRenameOp{}, paletteRmOp{},
		rampAddOp{}, rampRmOp{},
	} {
		Default.Register(op)
	}
}

// entryArg finds the palette entry that text names: a key, then a name, or
// either one forced with a key: or name: prefix (D53). Unlike color syntax it
// takes no ramp names, ramp steps or hex colors, so it never picks an entry
// by accident. The transparent entry is refused, because it is reserved.
// Errors name arg.
func entryArg(d *doc.Doc, arg, text string) (doc.Index, error) {
	p := &d.Palette
	i, ok := doc.Index(0), false
	switch {
	case strings.HasPrefix(text, "key:"):
		i, ok = p.ByKey(text[len("key:"):])
	case strings.HasPrefix(text, "name:"):
		i, ok = p.ByName(text[len("name:"):])
	case text == ".":
		i, ok = doc.Transparent, true
	default:
		if len(text) == d.KeyWidth {
			i, ok = p.ByKey(text)
		}
		if !ok {
			i, ok = p.ByName(text)
		}
	}
	switch {
	case ok && i == doc.Transparent:
		return 0, errs.New(errs.InvalidColor, "the transparent entry is reserved").
			WithHint("pick another entry by key or name").WithArg(arg)
	case ok:
		return i, nil
	case strings.HasPrefix(text, "#"):
		return 0, errs.New(errs.InvalidColor, "%q is a color, not a palette entry", text).
			WithHint("pick an entry by key or name; see `pixopher palette list`").WithArg(arg)
	}
	e := errs.New(errs.UnknownColor, "no palette entry %q", text).WithArg(arg)
	var cands []string
	for _, en := range p.Entries[1:] {
		cands = append(cands, en.Key, en.Name)
	}
	if best := color.Closest(strings.TrimPrefix(strings.TrimPrefix(text, "key:"), "name:"), cands); best != "" {
		e = e.WithHint("did you mean %q?", best)
	}
	return 0, e
}

// hexArg parses a #hex color for a palette entry. Entries may have any
// alpha, 0 included (D28).
func hexArg(arg, text string) (doc.RGBA, error) {
	if !strings.HasPrefix(text, "#") {
		return doc.RGBA{}, errs.New(errs.InvalidColor, "palette colors are #hex colors, got %q", text).
			WithHint("use #rgb, #rrggbb or #rrggbbaa").WithArg(arg)
	}
	c, err := color.ParseHex(text)
	if e, ok := errors.AsType[*errs.Error](err); ok {
		e.Arg = arg
	}
	return c, err
}

// nameArg checks a new entry or ramp name. Names don't depend on the
// document, so a bad one is a usage error (D27).
func nameArg(arg, name string) error {
	if doc.ValidName(name) {
		return nil
	}
	return errs.New(errs.Usage, "invalid name %q", name).
		WithHint("names are letters, digits and _, start with a letter or _, and are at most %d characters; transparent, none, key and name are reserved", doc.MaxNameLen).
		WithArg(arg)
}

// nameFree fails with already_exists when another entry than self is named
// name. Pass -1 as self for a new entry.
func nameFree(d *doc.Doc, arg, name string, self int) error {
	if i, ok := d.Palette.ByName(name); ok && int(i) != self {
		return errs.New(errs.AlreadyExists, "palette entry %q already exists", name).
			WithHint("pick another name, or rename the existing entry").WithArg(arg)
	}
	return nil
}

type paletteListOp struct{}

func (paletteListOp) Spec() Spec {
	return Spec{
		Name: "palette list", Group: "palette",
		Summary:  "List the palette entries (key, color, name) and the ramps",
		Examples: []string{"palette list", "palette list --json"},
	}
}

// PaletteReport is the result of palette list. Its JSON form is the --json
// output of `pxo palette list` (docs/palettes.md#palette-commands, D53).
type PaletteReport struct {
	KeyWidth int                    `json:"key_width"`
	Entries  []PaletteEntry         `json:"entries"` // in palette order, without transparent
	Ramps    map[string]PaletteRamp `json:"ramps"`
}

// PaletteEntry is one palette entry of a PaletteReport.
type PaletteEntry struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"` // #rrggbb, or #rrggbbaa when not opaque
}

// PaletteRamp is one ramp of a PaletteReport.
type PaletteRamp struct {
	Steps []string `json:"steps"` // keys, dark to light
	Base  int      `json:"base"`  // index into steps
}

// Observe lists the palette and ramps.
func (paletteListOp) Observe(d *doc.Doc, _ Args) (Report, error) {
	r := PaletteReport{KeyWidth: d.KeyWidth, Entries: []PaletteEntry{}, Ramps: map[string]PaletteRamp{}}
	for _, e := range d.Palette.Entries[1:] {
		r.Entries = append(r.Entries, PaletteEntry{Key: e.Key, Name: e.Name, Color: color.Hex(e.Color)})
	}
	for name, rp := range d.Ramps {
		steps := make([]string, len(rp.Steps))
		for i, s := range rp.Steps {
			steps[i] = d.Palette.Entries[s].Key
		}
		r.Ramps[name] = PaletteRamp{Steps: steps, Base: rp.Base}
	}
	return &r, nil
}

// Text is the human form: one line per entry, then one per ramp with its
// base step in brackets.
//
//	o  #1a1c2c  ink
//	g  #38b764  glass
//	ramp glass  o [g] G
func (r *PaletteReport) Text() string {
	var b strings.Builder
	if len(r.Entries) == 0 {
		b.WriteString("no colors\n")
	}
	cw := 0
	for _, e := range r.Entries {
		cw = max(cw, len(e.Color))
	}
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "%s  %-*s  %s\n", e.Key, cw, e.Color, e.Name)
	}
	width := 0
	for name := range r.Ramps {
		width = max(width, len(name))
	}
	for _, name := range doc.SortedKeys(r.Ramps) {
		rp := r.Ramps[name]
		steps := slices.Clone(rp.Steps)
		steps[rp.Base] = "[" + steps[rp.Base] + "]"
		fmt.Fprintf(&b, "ramp %-*s  %s\n", width, name, strings.Join(steps, " "))
	}
	return b.String()
}

type paletteAddOp struct{}

func (paletteAddOp) Spec() Spec {
	return Spec{
		Name: "palette add", Group: "palette", Summary: "Add a palette entry at the end", Mutates: true,
		Args: []ArgSpec{
			{Name: "key", Kind: KindString, Required: true, Help: "grid key, key_width characters long"},
			{Name: "color", Kind: KindColor, Required: true, Help: "a #hex color"},
			{Name: "name", Kind: KindString, Required: true, Help: "entry name"},
		},
		Examples: []string{"palette add k '#ffcc99' skin"},
	}
}

func (paletteAddOp) Apply(ctx *Ctx, args Args) error {
	d := ctx.Doc
	key, name := args.String("key"), args.String("name")
	if err := nameArg("name", name); err != nil {
		return err
	}
	if !doc.ValidKey(key, d.KeyWidth) {
		return errs.New(errs.InvalidColor, "invalid palette key %q", key).
			WithHint("keys are %d printable ASCII characters, not space or any of \" ' \\ # @ %% : + - , . _", d.KeyWidth).
			WithArg("key")
	}
	if i, ok := d.Palette.ByKey(key); ok {
		return errs.New(errs.AlreadyExists, "palette key %q is taken by %q", key, d.Palette.Entries[i].Name).
			WithHint("pick another key").WithArg("key")
	}
	if err := nameFree(d, "name", name, -1); err != nil {
		return err
	}
	c, err := hexArg("color", args.String("color"))
	if err != nil {
		return err
	}
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: key, Name: name, Color: c})
	return nil
}

type paletteSetOp struct{}

func (paletteSetOp) Spec() Spec {
	return Spec{
		Name: "palette set", Group: "palette", Mutates: true,
		Summary: "Change a palette entry's color; every pixel using it changes",
		Args: []ArgSpec{
			{Name: "entry", Kind: KindString, Required: true, Help: "entry key or name"},
			{Name: "color", Kind: KindColor, Required: true, Help: "a #hex color"},
		},
		Examples: []string{"palette set skin '#ffd0a0'"},
	}
}

func (paletteSetOp) Apply(ctx *Ctx, args Args) error {
	i, err := entryArg(ctx.Doc, "entry", args.String("entry"))
	if err != nil {
		return err
	}
	c, err := hexArg("color", args.String("color"))
	if err != nil {
		return err
	}
	ctx.Doc.Palette.Entries[i].Color = c
	return nil
}

type paletteRenameOp struct{}

func (paletteRenameOp) Spec() Spec {
	return Spec{
		Name: "palette rename", Group: "palette", Summary: "Rename a palette entry", Mutates: true,
		Args: []ArgSpec{
			{Name: "entry", Kind: KindString, Required: true, Help: "entry key or name"},
			{Name: "name", Kind: KindString, Required: true, Help: "new name"},
		},
		Examples: []string{"palette rename skin flesh"},
	}
}

// Apply renames the entry and the variant overrides that refer to it by
// name.
func (paletteRenameOp) Apply(ctx *Ctx, args Args) error {
	d := ctx.Doc
	i, err := entryArg(d, "entry", args.String("entry"))
	if err != nil {
		return err
	}
	name := args.String("name")
	if err := nameArg("name", name); err != nil {
		return err
	}
	if err := nameFree(d, "name", name, int(i)); err != nil {
		return err
	}
	old := d.Palette.Entries[i].Name
	if old == name {
		return nil
	}
	d.Palette.Entries[i].Name = name
	for _, v := range d.Variants {
		if c, ok := v.Entries[old]; ok {
			delete(v.Entries, old)
			v.Entries[name] = c
		}
	}
	return nil
}

type paletteRmOp struct{}

func (paletteRmOp) Spec() Spec {
	return Spec{
		Name: "palette rm", Group: "palette", Mutates: true,
		Summary: "Remove a palette entry; pixels using it need --replace",
		Args: []ArgSpec{
			{Name: "entry", Kind: KindString, Required: true, Help: "entry key or name"},
			{Name: "replace", Kind: KindColor, Flag: true, Help: "repaint pixels using the entry with this palette color"},
		},
		Examples: []string{"palette rm k", "palette rm auto_ff0000 --replace red"},
	}
}

// Apply removes the entry. Entries after it move down one index, so every
// pixel on every cel and part is renumbered, whatever layer it is on and
// whether that layer is locked: the document would not be valid otherwise
// (D53).
func (paletteRmOp) Apply(ctx *Ctx, args Args) error {
	d := ctx.Doc
	k, err := entryArg(d, "entry", args.String("entry"))
	if err != nil {
		return err
	}
	e := d.Palette.Entries[k]
	for _, name := range doc.SortedKeys(d.Ramps) {
		if slices.Contains(d.Ramps[name].Steps, k) {
			return errs.New(errs.InUse, "palette entry %q is a step of ramp %q", e.Name, name).
				WithHint("remove the ramp first: ramp rm %s", name).WithArg("entry")
		}
	}
	for _, name := range doc.SortedKeys(d.Variants) {
		if _, ok := d.Variants[name].Entries[e.Name]; ok {
			return errs.New(errs.InUse, "palette entry %q is overridden by variant %q", e.Name, name).WithArg("entry")
		}
	}
	repl := doc.Transparent
	if args.Has("replace") {
		r, err := color.Resolve(d, args.String("replace"))
		if ee, ok := errors.AsType[*errs.Error](err); ok {
			ee.Arg = "replace"
		}
		switch {
		case err != nil:
			return err
		case !r.InPalette:
			return errs.New(errs.UnknownColor, "%s is not in the palette", args.String("replace")).
				WithHint("add it with palette add, or change this entry's color with palette set").WithArg("replace")
		case r.Index == k:
			return errs.New(errs.InvalidColor, "--replace is the entry being removed").
				WithHint("pick another color").WithArg("replace")
		}
		repl = r.Index
	} else if n := uses(d, k); n > 0 {
		px := "pixels"
		if n == 1 {
			px = "pixel"
		}
		return errs.New(errs.InUse, "palette entry %q is used by %d %s", e.Name, n, px).
			WithHint("repaint them with --replace <color>, or erase them first").WithArg("entry")
	}
	if repl > k {
		repl--
	}
	renumber := func(v doc.Index) doc.Index {
		switch {
		case v == k:
			return repl
		case v > k:
			return v - 1
		}
		return v
	}
	for _, ck := range d.CelKeys() {
		c := d.Cels[ck]
		if c.Linked {
			continue // shares the pixels of an unlinked cel, renumbered once there
		}
		for p, v := range c.Pix {
			if v >= k {
				ctx.SetAt(ck.Layer, ck.Frame, p%d.W, p/d.W, renumber(v))
			}
		}
	}
	for _, part := range d.Parts {
		for p, v := range part.Pix {
			part.Pix[p] = renumber(v)
		}
	}
	for name, r := range d.Ramps {
		for s, v := range r.Steps {
			r.Steps[s] = renumber(v)
		}
		d.Ramps[name] = r
	}
	d.Palette.Entries = slices.Delete(d.Palette.Entries, int(k), int(k)+1)
	return nil
}

// uses counts the pixels of unlinked cels and parts that use entry k.
func uses(d *doc.Doc, k doc.Index) int {
	n := 0
	count := func(pix []doc.Index) {
		for _, v := range pix {
			if v == k {
				n++
			}
		}
	}
	for _, c := range d.Cels {
		if !c.Linked {
			count(c.Pix)
		}
	}
	for _, p := range d.Parts {
		count(p.Pix)
	}
	return n
}

type rampAddOp struct{}

func (rampAddOp) Spec() Spec {
	return Spec{
		Name: "ramp add", Group: "palette", Summary: "Add a ramp: palette entries ordered dark to light", Mutates: true,
		Args: []ArgSpec{
			{Name: "name", Kind: KindString, Required: true, Help: "ramp name"},
			{Name: "step", Kind: KindString, Required: true, Repeat: true, Help: "entry key or name, darkest first"},
			{Name: "base", Kind: KindString, Flag: true, Help: "the step used by the bare ramp name and by name+1 (default: the middle step)"},
		},
		Examples: []string{"ramp add skin S s t", "ramp add skin S s t --base t"},
	}
}

func (rampAddOp) Apply(ctx *Ctx, args Args) error {
	d := ctx.Doc
	name := args.String("name")
	if err := nameArg("name", name); err != nil {
		return err
	}
	if _, ok := d.Ramps[name]; ok {
		return errs.New(errs.AlreadyExists, "ramp %q already exists", name).
			WithHint("pick another name, or remove it first: ramp rm %s", name).WithArg("name")
	}
	texts, _ := args["step"].([]string)
	steps := make([]doc.Index, len(texts))
	for i, t := range texts {
		s, err := entryArg(d, "step", t)
		if err != nil {
			return err
		}
		if slices.Contains(steps[:i], s) {
			return errs.New(errs.InvalidColor, "palette entry %q is given twice", d.Palette.Entries[s].Name).
				WithHint("each entry can be one step of a ramp").WithArg("step")
		}
		steps[i] = s
	}
	base := doc.DefaultBase(len(steps))
	if args.Has("base") {
		b, err := entryArg(d, "base", args.String("base"))
		if err != nil {
			return err
		}
		if base = slices.Index(steps, b); base < 0 {
			return errs.New(errs.InvalidColor, "base %q is not a step of the ramp", args.String("base")).
				WithHint("pick one of the steps").WithArg("base")
		}
	}
	d.Ramps[name] = doc.Ramp{Steps: steps, Base: base}
	return nil
}

type rampRmOp struct{}

func (rampRmOp) Spec() Spec {
	return Spec{
		Name: "ramp rm", Group: "palette", Summary: "Remove a ramp; its palette entries stay", Mutates: true,
		Args: []ArgSpec{
			{Name: "name", Kind: KindString, Required: true, Help: "ramp name"},
		},
		Examples: []string{"ramp rm skin"},
	}
}

func (rampRmOp) Apply(ctx *Ctx, args Args) error {
	d := ctx.Doc
	name := args.String("name")
	if _, ok := d.Ramps[name]; !ok {
		e := errs.New(errs.UnknownColor, "no ramp named %q", name).WithArg("name")
		if best := color.Closest(name, doc.SortedKeys(d.Ramps)); best != "" {
			e = e.WithHint("did you mean %q?", best)
		}
		return e
	}
	for _, v := range doc.SortedKeys(d.Variants) {
		if _, ok := d.Variants[v].Ramps[name]; ok {
			return errs.New(errs.InUse, "ramp %q is overridden by variant %q", name, v).WithArg("name")
		}
	}
	delete(d.Ramps, name)
	return nil
}

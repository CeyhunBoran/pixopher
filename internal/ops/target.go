package ops

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// setSpec describes `@set`, the batch script directive that changes the
// target cel for the lines after it (docs/agent-workflow.md#batch-scripts).
// It is not in the registry: it is no command, and only scripts use it.
var setSpec = Spec{
	Name: "@set", Group: "script", Mutates: true,
	Summary: "Change the target layer and frame for the following lines",
	Args: []ArgSpec{
		{Name: "layer", Kind: KindString, Flag: true, Help: "layer name or index"},
		{Name: "frame", Kind: KindString, Flag: true, Help: "frame index, or tag:<name>"},
	},
}

type setOp struct{}

func (setOp) Spec() Spec { return setSpec }

// Apply moves ctx's target to the layer and frame in a, keeping the parts
// that a leaves out. It writes nothing.
func (setOp) Apply(ctx *Ctx, a Args) error {
	layer, frame := ctx.Layer, ctx.Frame
	if a.Has("layer") {
		v := a.String("layer")
		if n, err := strconv.Atoi(v); err == nil {
			if n >= len(ctx.Doc.Layers) {
				return errs.New(errs.UnknownLayer, "there is no layer %d", n).
					WithHint("layers are 0..%d", len(ctx.Doc.Layers)-1).WithArg("layer")
			}
			layer = n
		} else if layer = ctx.Doc.LayerIndex(v); layer < 0 {
			names := make([]string, len(ctx.Doc.Layers))
			for i, l := range ctx.Doc.Layers {
				names[i] = l.Name
			}
			return errs.New(errs.UnknownLayer, "there is no layer %q", v).
				WithHint("layers: %s", strings.Join(names, ", ")).WithArg("layer")
		}
	}
	if a.Has("frame") {
		v := a.String("frame")
		if tag, ok := strings.CutPrefix(v, "tag:"); ok {
			// Tags arrive with frames in M4; until then no document has one.
			return errs.New(errs.UnknownFrame, "there is no tag %q", tag).WithArg("frame")
		}
		n, _ := strconv.Atoi(v)
		if n >= len(ctx.Doc.Frames) {
			return errs.New(errs.UnknownFrame, "there is no frame %d", n).
				WithHint("frames are 0..%d", len(ctx.Doc.Frames)-1).WithArg("frame")
		}
		frame = n
	}
	ctx.Layer, ctx.Frame = layer, frame
	return nil
}

// ParseSet parses the `key=value` tokens after `@set` into a call. The keys
// are layer (a name or an index) and frame (an index or tag:<name>); at
// least one is needed and each at most once. Malformed tokens are usage
// errors. The call's Line is canonical: layer, then frame.
func ParseSet(tokens []string) (Call, error) {
	if len(tokens) == 0 {
		return Call{}, errs.New(errs.Usage, "@set needs layer=<name|index> or frame=<index>")
	}
	a := Args{}
	for _, t := range tokens {
		key, val, ok := strings.Cut(t, "=")
		switch {
		case !ok || key != "layer" && key != "frame":
			return Call{}, errs.New(errs.Usage, "@set takes layer=<name|index> and frame=<index>, not %q", t)
		case a.Has(key):
			return Call{}, errs.New(errs.Usage, "@set gives %s twice", key).WithArg(key)
		case key == "layer" && !isIndex(val) && !doc.ValidName(val):
			return Call{}, errs.New(errs.Usage, "layer=%q is not a layer name or index", val).WithArg(key)
		case key == "frame" && !isIndex(val) && !validTag(val):
			return Call{}, errs.New(errs.Usage, "frame=%q is not a frame index or tag:<name>", val).WithArg(key)
		}
		a[key] = val
	}
	return Call{Op: setOp{}, Args: a, Line: setLine(a)}, nil
}

// SetLine returns the @set line that targets the cel (layer, frame) of d,
// naming the layer.
func SetLine(d *doc.Doc, layer, frame int) string {
	return setLine(Args{"layer": d.Layers[layer].Name, "frame": strconv.Itoa(frame)})
}

func setLine(a Args) string {
	var b strings.Builder
	b.WriteString("@set")
	for _, k := range []string{"layer", "frame"} {
		if a.Has(k) {
			fmt.Fprintf(&b, " %s=%s", k, a.String(k))
		}
	}
	return b.String()
}

// IsSet reports whether c is an @set directive.
func IsSet(c Call) bool { _, ok := c.Op.(setOp); return ok }

// isIndex reports whether s is a non-negative decimal integer that fits an
// int.
func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func validTag(s string) bool {
	tag, ok := strings.CutPrefix(s, "tag:")
	return ok && doc.ValidName(tag)
}

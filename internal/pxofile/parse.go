package pxofile

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

func invalid(format string, a ...any) *errs.Error { return errs.New(errs.ParseError, format, a...) }

// Raw TOML shapes. Pointers mark optional keys whose absence means "default".
type (
	rawFile struct {
		Pixopher *int                      `toml:"pixopher"`
		Size     []int                     `toml:"size"`
		Rev      int                       `toml:"rev"`
		KeyWidth *int                      `toml:"key_width"`
		Meta     rawMeta                   `toml:"meta"`
		Palette  map[string][]string       `toml:"palette"`
		Ramps    map[string]any            `toml:"ramps"`
		Symmetry *rawSymmetry              `toml:"symmetry"`
		Layers   []rawLayer                `toml:"layers"`
		Frames   []rawFrame                `toml:"frames"`
		Tags     []rawTag                  `toml:"tags"`
		Regions  map[string][]int          `toml:"regions"`
		Anchors  map[string][]int          `toml:"anchors"`
		Variants map[string]map[string]any `toml:"variants"`
		Lint     rawLint                   `toml:"lint"`
		Parts    map[string]rawGrid        `toml:"parts"`
		Cels     []rawCel                  `toml:"cels"`
	}
	rawMeta struct {
		Title string `toml:"title"`
		// any, not time.Time: a time.Time field is filled through
		// UnmarshalText, which hides whether the value had an offset.
		Created any `toml:"created"`
	}
	rawSymmetry struct {
		Mode  *string  `toml:"mode"`
		AxisX *float64 `toml:"axis_x"`
		AxisY *float64 `toml:"axis_y"`
	}
	rawLayer struct {
		Name    string  `toml:"name"`
		Visible *bool   `toml:"visible"`
		Locked  bool    `toml:"locked"`
		Opacity *int    `toml:"opacity"`
		Blend   *string `toml:"blend"`
	}
	rawFrame struct {
		DurationMS *int             `toml:"duration_ms"`
		Anchors    map[string][]int `toml:"anchors"`
	}
	rawTag struct {
		Name      string  `toml:"name"`
		From      int     `toml:"from"`
		To        int     `toml:"to"`
		Direction *string `toml:"direction"`
	}
	rawLint struct {
		MaxColors int      `toml:"max_colors"`
		Disable   []string `toml:"disable"`
	}
	rawGrid struct {
		Grid     *string  `toml:"grid"`
		Encoding string   `toml:"encoding"`
		Rows     []string `toml:"rows"`
	}
	rawCel struct {
		rawGrid
		Layer  *string  `toml:"layer"`
		Frame  int      `toml:"frame"`
		Origin []int    `toml:"origin"`
		Link   *rawLink `toml:"link"`
	}
	rawLink struct {
		Frame *int `toml:"frame"`
	}
)

// Parse reads a .pxo document. Errors are *errs.Error with code parse_error,
// unsupported_version, too_large or grid_mismatch. Unknown keys are rejected,
// so rewriting a file never silently drops data.
func Parse(data []byte) (*doc.Doc, error) {
	var raw rawFile
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		if verr := checkVersionOnly(data); verr != nil {
			return nil, verr
		}
		return nil, tomlErr(err)
	}
	if raw.Pixopher == nil {
		return nil, invalid("missing `pixopher` format version").
			WithHint("the first line of a .pxo file is `pixopher = %d`", doc.FormatVersion)
	}
	if *raw.Pixopher != doc.FormatVersion {
		return nil, unsupported(*raw.Pixopher)
	}
	var unknown []string
	for _, k := range md.Undecoded() {
		// Keys inside a ramp table are checked by buildRamps.
		if len(k) == 3 && k[0] == "ramps" {
			continue
		}
		unknown = append(unknown, k.String())
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, invalid("unknown key %q", unknown[0]).
			WithHint("this build of pixopher may be older than the file; unknown keys are not dropped silently")
	}
	return build(&raw, md)
}

// checkVersionOnly reports unsupported_version for files whose body does not
// match this build's schema because they are a different major version.
func checkVersionOnly(data []byte) error {
	var v struct {
		Pixopher *int `toml:"pixopher"`
	}
	if _, err := toml.Decode(string(data), &v); err == nil && v.Pixopher != nil && *v.Pixopher != doc.FormatVersion {
		return unsupported(*v.Pixopher)
	}
	return nil
}

func unsupported(v int) error {
	return errs.New(errs.UnsupportedVersion, "unsupported format version %d", v).
		WithHint("this build reads pixopher = %d", doc.FormatVersion)
}

func tomlErr(err error) error {
	if pe, ok := errors.AsType[toml.ParseError](err); ok {
		return invalid("line %d: %s", pe.Position.Line, pe.Message)
	}
	return invalid("%s", err.Error())
}

func pair(what string, v []int) (int, int, error) {
	if len(v) != 2 {
		return 0, 0, invalid("%s must be [x, y], got %d values", what, len(v))
	}
	return v[0], v[1], nil
}

func build(raw *rawFile, md toml.MetaData) (*doc.Doc, error) {
	if len(raw.Size) != 2 {
		return nil, invalid("missing or malformed `size`").WithHint("write size = [width, height]")
	}
	d, err := doc.New(raw.Size[0], raw.Size[1])
	if err != nil {
		return nil, err
	}
	d.Rev = raw.Rev
	if raw.KeyWidth != nil {
		d.KeyWidth = *raw.KeyWidth
		if d.KeyWidth != 1 && d.KeyWidth != 2 {
			return nil, invalid("key_width must be 1 or 2, got %d", d.KeyWidth)
		}
		d.Palette = doc.NewPalette(d.KeyWidth)
	}
	d.Meta.Title = raw.Meta.Title
	if raw.Meta.Created != nil {
		c, ok := raw.Meta.Created.(time.Time)
		// The TOML library gives offset-less values these zone names. Their
		// meaning depends on the machine, so they are rejected.
		if loc := c.Location().String(); !ok || loc == "datetime-local" || loc == "date-local" || loc == "time-local" {
			return nil, invalid("meta.created must be a timestamp with a UTC offset").
				WithHint("write a full timestamp such as 2026-11-23T18:00:00Z")
		}
		d.Meta.Created = c.UTC()
	}
	steps := []func() error{
		func() error { return buildPalette(d, raw.Palette, md) },
		func() error { return buildRamps(d, raw.Ramps) },
		func() error { return buildSymmetry(d, raw.Symmetry) },
		func() error { return buildLayers(d, raw.Layers) },
		func() error { return buildFrames(d, raw.Frames) },
		func() error { return buildTags(d, raw.Tags) },
		func() error { return buildMarkers(d, raw.Regions, raw.Anchors) },
		func() error { return buildVariants(d, raw.Variants) },
		func() error { return buildLint(d, raw.Lint) },
		func() error { return buildParts(d, raw.Parts) },
		func() error { return buildCels(d, raw.Cels) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	d.PruneEmptyCels()
	return d, nil
}

// buildPalette keeps the file's key order (it defines palette indices). The
// transparent entry is always index 0, wherever it appears in the file.
func buildPalette(d *doc.Doc, pal map[string][]string, md toml.MetaData) error {
	tk := doc.TransparentKey(d.KeyWidth)
	for _, k := range md.Keys() {
		if len(k) != 2 || k[0] != "palette" {
			continue
		}
		key := k[1]
		v := pal[key]
		if len(v) != 2 {
			return invalid("palette entry %q must be [color, name]", key)
		}
		c, err := color.ParseHex(v[0])
		if err != nil {
			return invalid("palette entry %q: invalid color %q", key, v[0]).WithHint("use #rrggbb or #rrggbbaa")
		}
		if key == tk {
			if c != (doc.RGBA{}) || v[1] != doc.TransparentName {
				return invalid("palette entry %q is reserved and must be [\"#00000000\", %q]", tk, doc.TransparentName)
			}
			continue
		}
		d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: key, Name: v[1], Color: c})
	}
	return nil
}

func (dc *decoder) stepKeys(where string, v any) ([]doc.Index, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, invalid("%s must be an array of palette keys", where)
	}
	out := make([]doc.Index, len(arr))
	for i, s := range arr {
		k, ok := s.(string)
		if !ok {
			return nil, invalid("%s must be an array of palette keys", where)
		}
		idx, err := dc.key(where, k)
		if err != nil {
			return nil, err
		}
		out[i] = idx
	}
	return out, nil
}

// buildRamps accepts `name = ["a", "b"]` and `name = { steps = [...], base = "b" }`.
func buildRamps(d *doc.Doc, ramps map[string]any) error {
	dc := newDecoder(d)
	for _, name := range doc.SortedKeys(ramps) {
		where := fmt.Sprintf("ramp %q", name)
		var r doc.Ramp
		var base *string
		switch v := ramps[name].(type) {
		case []any:
			s, err := dc.stepKeys(where, v)
			if err != nil {
				return err
			}
			r.Steps = s
		case map[string]any:
			for k := range v {
				if k != "steps" && k != "base" {
					return invalid("%s: unknown key %q", where, k).WithHint("a ramp table has steps and base")
				}
			}
			s, err := dc.stepKeys(where+" steps", v["steps"])
			if err != nil {
				return err
			}
			r.Steps = s
			if b, ok := v["base"]; ok {
				bs, ok := b.(string)
				if !ok {
					return invalid("%s: base must be a palette key", where)
				}
				base = &bs
			}
		default:
			return invalid("%s must be an array of keys or { steps = [...], base = \"k\" }", where)
		}
		r.Base = doc.DefaultBase(len(r.Steps))
		if base != nil {
			idx, err := dc.key(where+" base", *base)
			if err != nil {
				return err
			}
			r.Base = -1
			for i, s := range r.Steps {
				if s == idx {
					r.Base = i
				}
			}
			if r.Base < 0 {
				return invalid("%s: base %q is not one of its steps", where, *base)
			}
		}
		d.Ramps[name] = r
	}
	return nil
}

func buildSymmetry(d *doc.Doc, s *rawSymmetry) error {
	if s == nil {
		return nil
	}
	if s.Mode != nil {
		d.Symmetry.Mode = *s.Mode
	}
	if s.AxisX != nil {
		d.Symmetry.AxisX = *s.AxisX
	}
	if s.AxisY != nil {
		d.Symmetry.AxisY = *s.AxisY
	}
	return nil
}

func buildLayers(d *doc.Doc, layers []rawLayer) error {
	if len(layers) == 0 {
		return nil // keep the default "main" layer
	}
	d.Layers = d.Layers[:0]
	for i, rl := range layers {
		l := doc.NewLayer(rl.Name)
		if rl.Visible != nil {
			l.Visible = *rl.Visible
		}
		l.Locked = rl.Locked
		if rl.Opacity != nil {
			if *rl.Opacity < 0 || *rl.Opacity > 255 {
				return invalid("layer %d opacity %d is outside 0..255", i, *rl.Opacity)
			}
			l.Opacity = uint8(*rl.Opacity)
		}
		if rl.Blend != nil {
			l.Blend = *rl.Blend
		}
		d.Layers = append(d.Layers, l)
	}
	return nil
}

func buildFrames(d *doc.Doc, frames []rawFrame) error {
	if len(frames) == 0 {
		return nil // keep the default frame
	}
	d.Frames = d.Frames[:0]
	for i, rf := range frames {
		f := doc.Frame{DurationMS: doc.DefaultDurationMS}
		if rf.DurationMS != nil {
			f.DurationMS = *rf.DurationMS
		}
		if len(rf.Anchors) > 0 {
			f.Anchors = make(map[string]doc.Point, len(rf.Anchors))
			for _, name := range doc.SortedKeys(rf.Anchors) {
				x, y, err := pair(fmt.Sprintf("frame %d anchor %q", i, name), rf.Anchors[name])
				if err != nil {
					return err
				}
				f.Anchors[name] = doc.Point{X: x, Y: y}
			}
		}
		d.Frames = append(d.Frames, f)
	}
	return nil
}

func buildTags(d *doc.Doc, tags []rawTag) error {
	for _, rt := range tags {
		t := doc.Tag{Name: rt.Name, From: rt.From, To: rt.To, Direction: doc.DefaultDirection}
		if rt.Direction != nil {
			t.Direction = *rt.Direction
		}
		d.Tags = append(d.Tags, t)
	}
	return nil
}

func buildMarkers(d *doc.Doc, regions, anchors map[string][]int) error {
	for _, name := range doc.SortedKeys(regions) {
		v := regions[name]
		if len(v) != 4 {
			return invalid("region %q must be [x, y, w, h], got %d values", name, len(v))
		}
		d.Regions[name] = doc.Rect{X: v[0], Y: v[1], W: v[2], H: v[3]}
	}
	for _, name := range doc.SortedKeys(anchors) {
		x, y, err := pair(fmt.Sprintf("anchor %q", name), anchors[name])
		if err != nil {
			return err
		}
		d.Anchors[name] = doc.Point{X: x, Y: y}
	}
	return nil
}

// buildVariants: an array value overrides a ramp, a string overrides one
// palette entry by name.
func buildVariants(d *doc.Doc, variants map[string]map[string]any) error {
	for _, name := range doc.SortedKeys(variants) {
		v := doc.Variant{Ramps: map[string][]doc.RGBA{}, Entries: map[string]doc.RGBA{}}
		for _, k := range doc.SortedKeys(variants[name]) {
			where := fmt.Sprintf("variant %q key %q", name, k)
			switch val := variants[name][k].(type) {
			case string:
				c, err := color.ParseHex(val)
				if err != nil {
					return invalid("%s: invalid color %q", where, val)
				}
				v.Entries[k] = c
			case []any:
				cs := make([]doc.RGBA, len(val))
				for i, x := range val {
					s, _ := x.(string)
					c, err := color.ParseHex(s)
					if err != nil {
						return invalid("%s: invalid color %v", where, x)
					}
					cs[i] = c
				}
				v.Ramps[k] = cs
			default:
				return invalid("%s must be a color or an array of colors", where)
			}
		}
		d.Variants[name] = v
	}
	return nil
}

func buildLint(d *doc.Doc, l rawLint) error {
	d.Lint.MaxColors = l.MaxColors
	if len(l.Disable) > 0 {
		dis := append([]string(nil), l.Disable...)
		sort.Strings(dis)
		d.Lint.Disable = dis
	}
	return nil
}

func (dc *decoder) grid(where string, g rawGrid) (pixGrid, error) {
	switch {
	case g.Encoding == "" && g.Grid != nil && g.Rows == nil:
		return dc.decodeGrid(where, *g.Grid)
	case g.Encoding == "rle" && g.Grid == nil && g.Rows != nil:
		return dc.decodeRLE(where, g.Rows)
	case g.Encoding != "" && g.Encoding != "rle":
		return pixGrid{}, invalid("%s: unknown encoding %q", where, g.Encoding).WithHint(`use encoding = "rle" or a grid`)
	}
	return pixGrid{}, invalid("%s needs either grid = '''...''' or encoding = \"rle\" with rows", where)
}

func buildParts(d *doc.Doc, parts map[string]rawGrid) error {
	dc := newDecoder(d)
	for _, name := range doc.SortedKeys(parts) {
		g, err := dc.grid(fmt.Sprintf("part %q", name), parts[name])
		if err != nil {
			return err
		}
		d.Parts[name] = doc.Part{W: g.W, H: g.H, Pix: g.Pix}
	}
	return nil
}

func buildCels(d *doc.Doc, cels []rawCel) error {
	dc := newDecoder(d)
	for _, rc := range cels {
		if rc.Layer == nil {
			return invalid("a cel has no layer")
		}
		li := d.LayerIndex(*rc.Layer)
		if li < 0 {
			return invalid("cel refers to unknown layer %q", *rc.Layer)
		}
		if rc.Frame < 0 || rc.Frame >= len(d.Frames) {
			return invalid("cel (layer %q) refers to frame %d, but there are %d frames", *rc.Layer, rc.Frame, len(d.Frames))
		}
		key := doc.CelKey{Layer: li, Frame: rc.Frame}
		where := fmt.Sprintf("cel (layer %q, frame %d)", *rc.Layer, rc.Frame)
		if d.Cels[key] != nil {
			return invalid("%s appears twice", where)
		}
		if rc.Link != nil {
			if rc.Link.Frame == nil || rc.Grid != nil || rc.Rows != nil || rc.Encoding != "" || rc.Origin != nil {
				return invalid("%s: a linked cel has only layer, frame and link = { frame = N }", where)
			}
			d.Cels[key] = &doc.Cel{Linked: true, LinkFrame: *rc.Link.Frame}
			continue
		}
		ox, oy := 0, 0
		if rc.Origin != nil {
			var err error
			if ox, oy, err = pair(where+" origin", rc.Origin); err != nil {
				return err
			}
		}
		g, err := dc.grid(where, rc.rawGrid)
		if err != nil {
			return err
		}
		if !(doc.Rect{X: ox, Y: oy, W: g.W, H: g.H}).In(d.W, d.H) {
			return mismatch("%s: %dx%d grid at [%d, %d] is not inside the %dx%d canvas", where, g.W, g.H, ox, oy, d.W, d.H)
		}
		pix := make([]doc.Index, d.W*d.H)
		for y := range g.H {
			copy(pix[(oy+y)*d.W+ox:], g.Pix[y*g.W:(y+1)*g.W])
		}
		d.Cels[key] = &doc.Cel{Pix: pix}
	}
	return nil
}

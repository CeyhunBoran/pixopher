package doc

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// MaxNameLen is the maximum length of a name.
const MaxNameLen = 64

// reservedWords can't be used as names: they are color syntax keywords.
var reservedWords = map[string]bool{"transparent": true, "none": true, "key": true, "name": true}

// ValidName reports whether s can name a palette entry, ramp, layer, tag,
// region, anchor, part or variant: an ASCII identifier ([A-Za-z_][A-Za-z0-9_]*)
// of at most MaxNameLen characters that is not a color keyword.
func ValidName(s string) bool {
	if s == "" || len(s) > MaxNameLen || reservedWords[s] {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9'
		if !ok {
			return false
		}
	}
	return true
}

func invalid(format string, a ...any) *errs.Error {
	return errs.New(errs.ParseError, format, a...)
}

func nameErr(what, name string) *errs.Error {
	return invalid("invalid %s name %q", what, name).
		WithHint("names are letters, digits and _, start with a letter or _, and are at most %d characters", MaxNameLen)
}

// Validate checks every invariant of the model. Violations come from
// malformed files, so they are reported as parse_error (size limits as
// too_large or out_of_bounds).
func (d *Doc) Validate() error {
	if err := checkSize(d.W, d.H); err != nil {
		return err
	}
	if d.Version != FormatVersion {
		return errs.New(errs.UnsupportedVersion, "unsupported format version %d", d.Version)
	}
	if d.Rev < 0 {
		return invalid("rev %d is negative", d.Rev)
	}
	if d.KeyWidth != 1 && d.KeyWidth != 2 {
		return invalid("key_width must be 1 or 2, got %d", d.KeyWidth)
	}
	checks := []func() error{
		d.validatePalette, d.validateRamps, d.validateSymmetry, d.validateLayers,
		d.validateFrames, d.validateTags, d.validateMarkers, d.validateVariants,
		d.validateLint, d.validateParts, d.validateCels,
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Doc) validatePalette() error {
	es := d.Palette.Entries
	if len(es) == 0 {
		return invalid("palette has no transparent entry")
	}
	if es[0] != (Entry{Key: TransparentKey(d.KeyWidth), Name: TransparentName}) {
		return invalid("palette entry 0 must be %q = [\"#00000000\", %q]", TransparentKey(d.KeyWidth), TransparentName)
	}
	if len(es) > math.MaxUint16+1 {
		return invalid("palette has %d entries", len(es))
	}
	keys := map[string]bool{es[0].Key: true}
	names := map[string]bool{TransparentName: true}
	for _, e := range es[1:] {
		if !ValidKey(e.Key, d.KeyWidth) {
			return invalid("invalid palette key %q for key_width %d", e.Key, d.KeyWidth).
				WithHint("keys are %d printable ASCII characters, none of %q", d.KeyWidth, reservedKeyChars)
		}
		if keys[e.Key] {
			return invalid("duplicate palette key %q", e.Key)
		}
		if !ValidName(e.Name) {
			return nameErr("palette entry", e.Name)
		}
		if names[e.Name] {
			return invalid("duplicate palette entry name %q", e.Name)
		}
		keys[e.Key], names[e.Name] = true, true
	}
	return nil
}

func (d *Doc) validateRamps() error {
	for _, name := range SortedKeys(d.Ramps) {
		r := d.Ramps[name]
		if !ValidName(name) {
			return nameErr("ramp", name)
		}
		if len(r.Steps) == 0 {
			return invalid("ramp %q has no steps", name)
		}
		seen := map[Index]bool{}
		for _, s := range r.Steps {
			if s == Transparent || int(s) >= d.Palette.Len() {
				return invalid("ramp %q has an invalid step", name)
			}
			if seen[s] {
				return invalid("ramp %q uses key %q twice", name, d.Palette.Entries[s].Key)
			}
			seen[s] = true
		}
		if r.Base < 0 || r.Base >= len(r.Steps) {
			return invalid("ramp %q base %d is outside 0..%d", name, r.Base, len(r.Steps)-1)
		}
	}
	return nil
}

func validAxis(a float64, size int) bool {
	return a >= 0 && a <= float64(size) && a*2 == math.Trunc(a*2)
}

func (d *Doc) validateSymmetry() error {
	s := d.Symmetry
	switch s.Mode {
	case SymOff, SymX, SymY, SymXY:
	default:
		return invalid("unknown symmetry mode %q", s.Mode).WithHint("use off, x, y or xy")
	}
	if !validAxis(s.AxisX, d.W) {
		return invalid("symmetry axis_x %v must be a multiple of 0.5 in 0..%d", s.AxisX, d.W)
	}
	if !validAxis(s.AxisY, d.H) {
		return invalid("symmetry axis_y %v must be a multiple of 0.5 in 0..%d", s.AxisY, d.H)
	}
	return nil
}

func (d *Doc) validateLayers() error {
	if len(d.Layers) == 0 {
		return invalid("document has no layers")
	}
	seen := map[string]bool{}
	for _, l := range d.Layers {
		if !ValidName(l.Name) {
			return nameErr("layer", l.Name)
		}
		if seen[l.Name] {
			return invalid("duplicate layer name %q", l.Name)
		}
		seen[l.Name] = true
		if l.Blend == "" {
			return invalid("layer %q has an empty blend mode", l.Name)
		}
	}
	return nil
}

func (d *Doc) validateFrames() error {
	if len(d.Frames) == 0 {
		return invalid("document has no frames")
	}
	for i, f := range d.Frames {
		if f.DurationMS < 1 {
			return invalid("frame %d duration_ms must be at least 1, got %d", i, f.DurationMS)
		}
		for _, name := range SortedKeys(f.Anchors) {
			if _, ok := d.Anchors[name]; !ok {
				return invalid("frame %d overrides anchor %q, which is not in [anchors]", i, name)
			}
			if p := f.Anchors[name]; !d.inCanvas(p) {
				return invalid("frame %d anchor %q [%d, %d] is outside the canvas", i, name, p.X, p.Y)
			}
		}
	}
	return nil
}

func (d *Doc) validateTags() error {
	seen := map[string]bool{}
	for _, t := range d.Tags {
		if !ValidName(t.Name) {
			return nameErr("tag", t.Name)
		}
		if seen[t.Name] {
			return invalid("duplicate tag %q", t.Name)
		}
		seen[t.Name] = true
		if t.From < 0 || t.From > t.To || t.To >= len(d.Frames) {
			return invalid("tag %q range %d..%d is invalid for %d frames", t.Name, t.From, t.To, len(d.Frames))
		}
		switch t.Direction {
		case DirForward, DirReverse, DirPingpong:
		default:
			return invalid("tag %q has unknown direction %q", t.Name, t.Direction).
				WithHint("use forward, reverse or pingpong")
		}
	}
	return nil
}

func (d *Doc) inCanvas(p Point) bool { return p.X >= 0 && p.Y >= 0 && p.X < d.W && p.Y < d.H }

func (d *Doc) validateMarkers() error {
	for _, name := range SortedKeys(d.Regions) {
		r := d.Regions[name]
		if !ValidName(name) {
			return nameErr("region", name)
		}
		if !r.In(d.W, d.H) {
			return invalid("region %q [%d, %d, %d, %d] is not inside the canvas", name, r.X, r.Y, r.W, r.H)
		}
	}
	for _, name := range SortedKeys(d.Anchors) {
		p := d.Anchors[name]
		if !ValidName(name) {
			return nameErr("anchor", name)
		}
		if !d.inCanvas(p) {
			return invalid("anchor %q [%d, %d] is outside the canvas", name, p.X, p.Y)
		}
	}
	return nil
}

func (d *Doc) validateVariants() error {
	for _, name := range SortedKeys(d.Variants) {
		v := d.Variants[name]
		if !ValidName(name) {
			return nameErr("variant", name)
		}
		for _, r := range SortedKeys(v.Ramps) {
			ramp, ok := d.Ramps[r]
			if !ok {
				return invalid("variant %q overrides unknown ramp %q", name, r)
			}
			if len(v.Ramps[r]) != len(ramp.Steps) {
				return invalid("variant %q gives %d colors for ramp %q, which has %d steps",
					name, len(v.Ramps[r]), r, len(ramp.Steps))
			}
		}
		for _, e := range SortedKeys(v.Entries) {
			if i, ok := d.Palette.ByName(e); !ok || i == Transparent {
				return invalid("variant %q overrides unknown palette entry %q", name, e)
			}
		}
	}
	return nil
}

func (d *Doc) validateLint() error {
	if d.Lint.MaxColors < 0 {
		return invalid("lint max_colors %d is negative", d.Lint.MaxColors)
	}
	if !sort.StringsAreSorted(d.Lint.Disable) {
		return invalid("lint disable list is not sorted")
	}
	for i, r := range d.Lint.Disable {
		if r == "" || i > 0 && r == d.Lint.Disable[i-1] {
			return invalid("lint disable list has an empty or duplicate rule")
		}
	}
	return nil
}

func (d *Doc) checkPix(what string, pix []Index, n int) error {
	if len(pix) != n {
		return invalid("%s has %d pixels, want %d", what, len(pix), n)
	}
	for _, p := range pix {
		if int(p) >= d.Palette.Len() {
			return invalid("%s uses palette index %d, which does not exist", what, p)
		}
	}
	return nil
}

func (d *Doc) validateParts() error {
	for _, name := range SortedKeys(d.Parts) {
		p := d.Parts[name]
		if !ValidName(name) {
			return nameErr("part", name)
		}
		if p.W < 1 || p.H < 1 || p.W > MaxSize || p.H > MaxSize {
			return invalid("part %q size %dx%d is invalid", name, p.W, p.H)
		}
		if err := d.checkPix(fmt.Sprintf("part %q", name), p.Pix, p.W*p.H); err != nil {
			return err
		}
	}
	return nil
}

func (d *Doc) validateCels() error {
	for _, k := range d.CelKeys() {
		c := d.Cels[k]
		if k.Layer < 0 || k.Layer >= len(d.Layers) || k.Frame < 0 || k.Frame >= len(d.Frames) {
			return invalid("cel (layer %d, frame %d) does not exist", k.Layer, k.Frame)
		}
		what := fmt.Sprintf("cel (layer %q, frame %d)", d.Layers[k.Layer].Name, k.Frame)
		if c == nil {
			return invalid("%s is nil", what)
		}
		if c.Linked {
			if c.Pix != nil {
				return invalid("%s is linked but has pixels", what)
			}
			if c.LinkFrame == k.Frame {
				return invalid("%s links to itself", what)
			}
			t := d.Cels[CelKey{k.Layer, c.LinkFrame}]
			if t == nil || t.Linked {
				return invalid("%s links to frame %d, which has no unlinked cel on this layer", what, c.LinkFrame)
			}
			continue
		}
		if err := d.checkPix(what, c.Pix, d.W*d.H); err != nil {
			return err
		}
	}
	return nil
}

// CelKeys returns the keys of all cels, sorted by layer, then frame.
func (d *Doc) CelKeys() []CelKey {
	keys := make([]CelKey, 0, len(d.Cels))
	for k := range d.Cels {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Layer != keys[j].Layer {
			return keys[i].Layer < keys[j].Layer
		}
		return keys[i].Frame < keys[j].Frame
	})
	return keys
}

// SortedKeys returns the keys of m in sorted order. Writers and anything that
// iterates over a map for output use it to stay deterministic.
func SortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

package render

import (
	"fmt"
	"image"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Composite flattens frame of d: its visible layers, bottom first, are
// drawn over a transparent canvas with source-over, each pixel's alpha
// multiplied by its layer's opacity. Blend modes other than normal aren't
// implemented yet; such layers are drawn as normal and named in the
// returned warnings (D58). The math is integer and exact, so the result is
// the same on every platform, and a single opaque layer gives back exactly
// its palette colors. A fully transparent result pixel is always 0,0,0,0.
func Composite(d *doc.Doc, frame int) (*image.NRGBA, []string) {
	img := image.NewNRGBA(image.Rect(0, 0, d.W, d.H))
	var warnings []string
	for li, l := range d.Layers {
		if !l.Visible || l.Opacity == 0 {
			continue
		}
		pix := d.Pixels(li, frame)
		if pix == nil {
			continue
		}
		if l.Blend != doc.DefaultBlend {
			warnings = append(warnings,
				fmt.Sprintf("layer %q: blend mode %q is not supported yet; drawn as normal", l.Name, l.Blend))
		}
		entries := d.Palette.Entries
		for i, v := range pix {
			if v == doc.Transparent || int(v) >= len(entries) {
				continue
			}
			c := entries[v].Color
			sa := c.A
			if l.Opacity != 255 {
				sa = uint8((uint32(c.A)*uint32(l.Opacity) + 127) / 255)
			}
			over(img.Pix[i*4:i*4+4:i*4+4], c, sa)
		}
	}
	return img, warnings
}

// over draws color c with alpha sa over the non-premultiplied pixel dst.
func over(dst []byte, c doc.RGBA, sa uint8) {
	switch {
	case sa == 0:
		return
	case sa == 255 || dst[3] == 0:
		dst[0], dst[1], dst[2], dst[3] = c.R, c.G, c.B, sa
		return
	}
	s, da := uint32(sa), uint32(dst[3])
	// Both alphas scaled by 255: a = sa + da*(1-sa), and each channel is
	// (c*sa + d*da*(1-sa)) / a, rounded to nearest.
	db := da * (255 - s)
	a := s*255 + db
	mix := func(sc, dc uint8) uint8 {
		return uint8((uint32(sc)*s*255 + uint32(dc)*db + a/2) / a)
	}
	dst[0], dst[1], dst[2] = mix(c.R, dst[0]), mix(c.G, dst[1]), mix(c.B, dst[2])
	dst[3] = uint8((a + 127) / 255)
}

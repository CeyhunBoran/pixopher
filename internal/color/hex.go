// Package color parses colors and implements OKLab/OKLCH math, ramp generation,
// quantization and color difference.
package color

import (
	"fmt"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// ParseHex parses "#rgb", "#rrggbb" or "#rrggbbaa" (any case).
func ParseHex(s string) (doc.RGBA, error) {
	bad := func() error {
		return errs.New(errs.InvalidColor, "invalid hex color %q", s).
			WithHint("use #rgb, #rrggbb or #rrggbbaa")
	}
	if len(s) < 1 || s[0] != '#' {
		return doc.RGBA{}, bad()
	}
	h := s[1:]
	var v [8]byte
	for i := 0; i < len(h); i++ {
		n, ok := hexDigit(h[i])
		if !ok || i >= len(v) {
			return doc.RGBA{}, bad()
		}
		v[i] = n
	}
	switch len(h) {
	case 3:
		return doc.RGBA{R: v[0] * 17, G: v[1] * 17, B: v[2] * 17, A: 0xff}, nil
	case 6:
		return doc.RGBA{R: v[0]<<4 | v[1], G: v[2]<<4 | v[3], B: v[4]<<4 | v[5], A: 0xff}, nil
	case 8:
		return doc.RGBA{R: v[0]<<4 | v[1], G: v[2]<<4 | v[3], B: v[4]<<4 | v[5], A: v[6]<<4 | v[7]}, nil
	}
	return doc.RGBA{}, bad()
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// Hex formats c as lowercase "#rrggbb", or "#rrggbbaa" when it is not opaque.
func Hex(c doc.RGBA) string {
	if c.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

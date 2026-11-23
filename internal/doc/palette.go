package doc

import (
	"fmt"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// TransparentName is the name of the reserved "." entry.
const TransparentName = "transparent"

// Reserved key characters (docs/file-format.md#palette-keys). They can never
// be part of a key. "." only appears as the transparent key ("." or "..").
const reservedKeyChars = " \"'\\#@%:+-,._"

// autoKeyOrder is the order in which generated keys are tried: letters,
// digits, then the remaining printable ASCII punctuation in ASCII order.
var autoKeyOrder = func() string {
	var b strings.Builder
	b.WriteString("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	for c := byte('!'); c <= '~'; c++ {
		if isKeyChar(c) && !isAlnum(c) {
			b.WriteByte(c)
		}
	}
	return b.String()
}()

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isKeyChar reports whether c may appear in a non-transparent key.
func isKeyChar(c byte) bool {
	return c > ' ' && c <= '~' && strings.IndexByte(reservedKeyChars, c) < 0
}

// TransparentKey returns the transparent key for a key width: "." or "..".
func TransparentKey(keyWidth int) string { return strings.Repeat(".", keyWidth) }

// EraseKey returns the grid-paint erase marker for a key width: "_" or "__".
// It is never stored in a cel.
func EraseKey(keyWidth int) string { return strings.Repeat("_", keyWidth) }

// ValidKey reports whether k is a usable key for a non-transparent entry at
// the given key width.
func ValidKey(k string, keyWidth int) bool {
	if len(k) != keyWidth {
		return false
	}
	for i := 0; i < len(k); i++ {
		if !isKeyChar(k[i]) {
			return false
		}
	}
	return true
}

// Entry is one palette entry.
type Entry struct {
	Key   string
	Name  string
	Color RGBA
}

// Palette is an ordered list of entries. Order is meaningful and preserved.
// Entries[0] is always the transparent entry.
type Palette struct {
	Entries []Entry
}

// NewPalette returns a palette holding only the transparent entry.
func NewPalette(keyWidth int) Palette {
	return Palette{Entries: []Entry{{Key: TransparentKey(keyWidth), Name: TransparentName}}}
}

// Len returns the number of entries, the transparent one included.
func (p *Palette) Len() int { return len(p.Entries) }

// ByKey returns the index of the entry with key k.
func (p *Palette) ByKey(k string) (Index, bool) {
	for i, e := range p.Entries {
		if e.Key == k {
			return Index(i), true
		}
	}
	return 0, false
}

// ByName returns the index of the entry named n.
func (p *Palette) ByName(n string) (Index, bool) {
	for i, e := range p.Entries {
		if e.Name == n {
			return Index(i), true
		}
	}
	return 0, false
}

// ByColor returns the index of the first non-transparent entry whose color is
// exactly c.
func (p *Palette) ByColor(c RGBA) (Index, bool) {
	for i := 1; i < len(p.Entries); i++ {
		if p.Entries[i].Color == c {
			return Index(i), true
		}
	}
	return 0, false
}

// AutoEntry returns the entry that drawing with the raw color c would add: the
// first free key in a fixed order, and a name like "auto_3f2a1b" (with the
// alpha appended when it is not ff). It does not modify the palette, so the
// caller can record the addition as an undoable change.
//
// The palette is full when every key of the current width is taken. Widening
// a document to key_width 2 is not automatic yet.
func (p *Palette) AutoEntry(c RGBA, keyWidth int) (Entry, error) {
	key, ok := p.freeKey(keyWidth)
	if !ok {
		return Entry{}, errs.New(errs.PaletteFull, "palette is full: no free %d-character keys", keyWidth).
			WithHint("reuse an existing color, or remove unused entries")
	}
	base := fmt.Sprintf("auto_%02x%02x%02x", c.R, c.G, c.B)
	if c.A != 0xff {
		base += fmt.Sprintf("%02x", c.A)
	}
	name := base
	for n := 2; ; n++ {
		if _, taken := p.ByName(name); !taken {
			break
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
	return Entry{Key: key, Name: name, Color: c}, nil
}

func (p *Palette) freeKey(keyWidth int) (string, bool) {
	used := make(map[string]bool, len(p.Entries))
	for _, e := range p.Entries {
		used[e.Key] = true
	}
	switch keyWidth {
	case 1:
		for i := 0; i < len(autoKeyOrder); i++ {
			if k := autoKeyOrder[i : i+1]; !used[k] {
				return k, true
			}
		}
	case 2:
		for i := 0; i < len(autoKeyOrder); i++ {
			for j := 0; j < len(autoKeyOrder); j++ {
				if k := string([]byte{autoKeyOrder[i], autoKeyOrder[j]}); !used[k] {
					return k, true
				}
			}
		}
	}
	return "", false
}

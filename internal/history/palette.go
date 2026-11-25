package history

import (
	"slices"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// Palette-aware undo (D54). Palette entries are matched by key, not index:
// a step only checks and restores the entries it changed, and pixel indices
// are carried from the palette they were written against to the document's
// palette through keys.

// keyIndex maps the keys of a palette to their indices.
func keyIndex(es []doc.Entry) map[string]doc.Index {
	m := make(map[string]doc.Index, len(es))
	for i, e := range es {
		m[e.Key] = doc.Index(i)
	}
	return m
}

// translator carries palette indices from one palette to another through
// keys.
type translator struct {
	from []doc.Entry
	to   map[string]doc.Index
	same bool // the palettes are equal, so indices carry over as they are
}

func newTranslator(from, to []doc.Entry) translator {
	return translator{from: from, to: keyIndex(to), same: slices.Equal(from, to)}
}

// identity is a translator that keeps every index.
var identity = translator{same: true}

// index returns the index in the target palette of the entry v is in the
// source palette, and false when the target has no entry with its key.
func (tr translator) index(v doc.Index) (doc.Index, bool) {
	if tr.same || v == doc.Transparent {
		return v, true
	}
	if int(v) >= len(tr.from) {
		return 0, false
	}
	i, ok := tr.to[tr.from[v].Key]
	return i, ok
}

// entry returns the entry with key k in es.
func entry(es []doc.Entry, k string) (doc.Entry, bool) {
	i := slices.IndexFunc(es, func(e doc.Entry) bool { return e.Key == k })
	if i < 0 {
		return doc.Entry{}, false
	}
	return es[i], true
}

// sameEntry reports whether a and b hold the same entry for key k, or both
// have none.
func sameEntry(a, b []doc.Entry, k string) bool {
	ea, oka := entry(a, k)
	eb, okb := entry(b, k)
	return oka == okb && ea == eb
}

// touchedKeys returns the keys whose entry differs between palettes a and b
// (added, removed, renamed or recolored): a's order first, then the keys
// only b has, in b's order. reordered reports whether the keys both have
// appear in a different order.
func touchedKeys(a, b []doc.Entry) (keys []string, reordered bool) {
	ai, bi := keyIndex(a), keyIndex(b)
	var sa, sb []string
	for _, e := range a {
		j, ok := bi[e.Key]
		if !ok || b[j] != e {
			keys = append(keys, e.Key)
		}
		if ok {
			sa = append(sa, e.Key)
		}
	}
	for _, e := range b {
		if _, ok := ai[e.Key]; ok {
			sb = append(sb, e.Key)
		} else {
			keys = append(keys, e.Key)
		}
	}
	return keys, !slices.Equal(sa, sb)
}

// mergePalette applies a step's palette change to cur, key by key: each
// touched key takes its entry in to, or is removed when to has none. An
// entry cur lacks is inserted after the closest entry before it in to that
// cur has, so undoing and redoing an addition puts it back where it was.
// Entries the step didn't touch keep their order.
func mergePalette(cur, to []doc.Entry, touched []string) []doc.Entry {
	out := slices.Clone(cur)
	ti := keyIndex(to)
	for _, k := range touched {
		if _, ok := ti[k]; !ok {
			out = slices.DeleteFunc(out, func(e doc.Entry) bool { return e.Key == k })
		}
	}
	for i, e := range to {
		if !slices.Contains(touched, e.Key) {
			continue
		}
		if j := slices.IndexFunc(out, func(x doc.Entry) bool { return x.Key == e.Key }); j >= 0 {
			out[j] = e
			continue
		}
		at := 1 // after transparent
		for _, prev := range slices.Backward(to[:i]) {
			if j := slices.IndexFunc(out, func(x doc.Entry) bool { return x.Key == prev.Key }); j >= 0 {
				at = j + 1
				break
			}
		}
		out = slices.Insert(out, at, e)
	}
	return out
}

// headerPlan is how a step with a meta delta checks and restores the header.
type headerPlan struct {
	want, to *doc.Doc // what the document holds if nothing changed since, and what the step restores

	// whole compares and restores the whole header, indices as they are:
	// for key width changes and reordered entries, where keys can't match
	// entries up.
	whole    bool
	touched  []string // palette keys the step changes
	rest     bool     // the step changes the header beyond the palette
	wantRest string   // HeaderRest(want), when rest
}

func planHeader(want, to, cur *doc.Doc) (*headerPlan, error) {
	p := &headerPlan{want: want, to: to}
	var reordered bool
	p.touched, reordered = touchedKeys(want.Palette.Entries, to.Palette.Entries)
	if reordered || want.KeyWidth != to.KeyWidth || cur.KeyWidth != want.KeyWidth {
		p.whole = true
		return p, nil
	}
	w, err := pxofile.HeaderRest(want)
	if err != nil {
		return nil, err
	}
	t, err := pxofile.HeaderRest(to)
	if err != nil {
		return nil, err
	}
	p.rest, p.wantRest = w != t, w
	return p, nil
}

// changedKeys returns the touched keys whose entry in cur is no longer the
// one the step left.
func (p *headerPlan) changedKeys(cur []doc.Entry) []string {
	var out []string
	for _, k := range p.touched {
		if !sameEntry(cur, p.want.Palette.Entries, k) {
			out = append(out, k)
		}
	}
	return out
}

// clash returns the first entry the step restores whose key or name another
// entry has taken since, and that entry. Restoring it would recolor that
// entry's pixels, so not even --force does it.
func (p *headerPlan) clash(cur []doc.Entry) (restored, taken doc.Entry, ok bool) {
	for _, k := range p.touched {
		e, has := entry(p.to.Palette.Entries, k)
		if !has {
			continue
		}
		if _, before := entry(p.want.Palette.Entries, k); !before {
			if c, taken := entry(cur, k); taken && c != e {
				return e, c, true
			}
		}
		for _, c := range cur {
			if c.Name == e.Name && c.Key != k && !slices.Contains(p.touched, c.Key) {
				return e, c, true
			}
		}
	}
	return doc.Entry{}, doc.Entry{}, false
}

// paletteAt returns the palette the pixel deltas of the entry at rev refer
// to, for an entry without a meta delta: the palette before the next entry
// in the chain that changed the header, or start (the document's palette
// before this undo or redo) when none did.
func (c Chain) paletteAt(rev int, start []doc.Entry) ([]doc.Entry, error) {
	i := slices.IndexFunc(c.Entries, func(e pxofile.Entry) bool { return e.Rev == rev })
	for _, e := range c.Entries[i+1:] {
		if len(e.Undo.Meta) == 0 {
			continue
		}
		m, err := pxofile.DecodeMeta(e.Undo.Meta)
		if err != nil {
			return nil, err
		}
		return m.Before.Palette.Entries, nil
	}
	return start, nil
}

// headerHit is the part of the header an undo conflicts on, for blame.
type headerHit struct {
	whole bool     // the whole header
	keys  []string // these palette entries
	rest  bool     // the header beyond the palette
}

func (h headerHit) any() bool { return h.whole || len(h.keys) > 0 || h.rest }

// touches reports whether the meta delta of e changed the part of the
// header h names. An entry whose delta can't be read counts.
func (h headerHit) touches(e pxofile.Entry) bool {
	if len(e.Undo.Meta) == 0 || !h.any() {
		return false
	}
	if h.whole {
		return true
	}
	m, err := pxofile.DecodeMeta(e.Undo.Meta)
	if err != nil {
		return true
	}
	keys, reordered := touchedKeys(m.Before.Palette.Entries, m.After.Palette.Entries)
	if reordered || slices.ContainsFunc(keys, func(k string) bool { return slices.Contains(h.keys, k) }) {
		return true
	}
	if h.rest {
		b, err1 := pxofile.HeaderRest(m.Before)
		a, err2 := pxofile.HeaderRest(m.After)
		return err1 != nil || err2 != nil || a != b
	}
	return false
}

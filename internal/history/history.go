package history

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// Chain is the effective history of a document: the log entries that led to
// its current rev, oldest first (D49). Entries above the document's rev, and
// entries abandoned when a rollback made revs repeat, are not part of it.
type Chain struct {
	Entries []pxofile.Entry
	// Warnings say when the log and the document are out of step.
	Warnings []string

	undone map[int]bool // rev of a normal entry → currently undone
}

// Build returns the chain of a document at rev from its log entries in file
// order. It reads the log backward from its end and keeps an entry only
// when its rev is below the last one kept, starting below rev+1. Revs in
// the chain therefore strictly grow, and a tx id names one transaction.
func Build(entries []pxofile.Entry, rev int) Chain {
	var c Chain
	bound, ahead := rev+1, 0
	for _, e := range slices.Backward(entries) {
		switch {
		case e.Rev >= 1 && e.Rev < bound:
			c.Entries = append(c.Entries, e)
			bound = e.Rev
		case len(c.Entries) == 0 && e.Rev > rev:
			ahead++
		}
	}
	slices.Reverse(c.Entries)
	if ahead > 0 {
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"the history log has %d %s above the document's rev %d; %s ignored",
			ahead, plural(ahead, "entry", "entries"), rev, plural(ahead, "it is", "they are")))
	}
	last := 0
	if n := len(c.Entries); n > 0 {
		last = c.Entries[n-1].Rev
	}
	if last < rev {
		what := "has no history"
		if last > 0 {
			what = fmt.Sprintf("its history ends at rev %d", last)
		}
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"the document is at rev %d but %s; later changes can't be undone", rev, what))
	}
	c.undone = map[int]bool{}
	normal := map[int]bool{}
	for _, e := range c.Entries {
		switch e.Kind {
		case "":
			normal[e.Rev] = true
		case pxofile.KindUndo, pxofile.KindRedo:
			if normal[e.Target] {
				c.undone[e.Target] = e.Kind == pxofile.KindUndo
			}
		}
	}
	return c
}

// Undone reports whether the normal entry at rev is currently undone.
func (c Chain) Undone(rev int) bool { return c.undone[rev] }

// Request says what to undo or redo (docs/cli.md#history-m1-m7).
type Request struct {
	Redo   bool   // redo instead of undo
	N      int    // how many transactions, or entries with Step; at least 1
	Step   bool   // count entries instead of transactions
	Any    bool   // undo anyone's transactions, not only Author's
	Force  bool   // revert pixels and headers that changed since
	Author string // who is undoing or redoing
}

// Step is one entry reverted (undo) or applied again (redo).
type Step struct {
	Target pxofile.Entry // the normal entry
	Change ops.Change    // what this step did to the document
}

// Conflict is the details of an undo_conflict caused by later changes.
type Conflict struct {
	Target int      `json:"target"` // rev of the entry being undone or redone
	Count  int      `json:"count"`  // pixels that changed since
	Pixels [][4]int `json:"pixels"` // the first MaxConflictPixels of them as [layer, frame, x, y], sorted (D51)
	Meta   bool     `json:"meta,omitempty"`
	Txs    []string `json:"txs"` // later transactions that changed them, oldest first
}

// MaxConflictPixels caps Conflict.Pixels.
const MaxConflictPixels = 64

// Run picks the entries req asks for and reverts or re-applies them on d,
// one Step per entry, in order. Before each step its deltas are checked
// against d (D49, D54): a delta that doesn't fit the document (canvas,
// layer or frame), a palette entry whose key or name was taken since, an
// entry still used elsewhere that the step removes, an entry removed since
// that the step restores, and a result that doesn't validate fail with
// undo_conflict even with Force; pixels, palette entries or the rest of the
// header that changed since fail unless Force is set. On error d is left half-changed and must be discarded.
func Run(d *doc.Doc, c Chain, req Request) ([]Step, error) {
	targets, err := c.targets(req)
	if err != nil {
		return nil, err
	}
	start := slices.Clone(d.Palette.Entries)
	steps := make([]Step, 0, len(targets))
	for _, t := range targets {
		ch, err := c.apply(d, t, req, start)
		if err != nil {
			return nil, err
		}
		steps = append(steps, Step{Target: t, Change: ch})
	}
	return steps, nil
}

// targets returns the entries to revert or re-apply, in order.
func (c Chain) targets(req Request) ([]pxofile.Entry, error) {
	if req.Redo {
		return c.redoTargets(req)
	}
	undone := maps.Clone(c.undone)
	txs := c.txs()
	var out []pxofile.Entry
	for k := range req.N {
		var picked []pxofile.Entry
		for _, tx := range slices.Backward(txs) {
			first := c.Entries[tx[0]]
			if first.Kind != "" || !req.Any && first.Author != req.Author {
				continue
			}
			for _, i := range slices.Backward(tx) {
				if e := c.Entries[i]; !undone[e.Rev] {
					picked = append(picked, e)
				}
			}
			if len(picked) > 0 {
				break
			}
		}
		if len(picked) == 0 {
			return nil, c.nothingTo(req, k)
		}
		if req.Step {
			picked = picked[:1]
		}
		for _, e := range picked {
			undone[e.Rev] = true
		}
		out = append(out, picked...)
	}
	return out, nil
}

// redoTargets pops the author's redo stack: the entries the author undid,
// most recent first, cleared by each new normal entry of theirs. Without
// Step, one count re-applies the top entry and the entries of its
// transaction below it.
func (c Chain) redoTargets(req Request) ([]pxofile.Entry, error) {
	byRev := map[int]pxofile.Entry{}
	var stack []pxofile.Entry
	for _, e := range c.Entries {
		switch {
		case e.Kind == "":
			byRev[e.Rev] = e
			if e.Author == req.Author {
				stack = nil
			}
		case e.Author != req.Author:
		case e.Kind == pxofile.KindUndo:
			if t, ok := byRev[e.Target]; ok {
				stack = append(stack, t)
			}
		case e.Kind == pxofile.KindRedo:
			for i, t := range slices.Backward(stack) {
				if t.Rev == e.Target {
					stack = slices.Delete(stack, i, i+1)
					break
				}
			}
		}
	}
	stack = slices.DeleteFunc(stack, func(t pxofile.Entry) bool { return !c.undone[t.Rev] })
	var out []pxofile.Entry
	for k := range req.N {
		if len(stack) == 0 {
			return nil, c.nothingTo(req, k)
		}
		top := len(stack) - 1
		from := top
		if !req.Step {
			// A transaction counts once, however many undos reverted it.
			for from > 0 && stack[from-1].Tx == stack[top].Tx {
				from--
			}
		}
		for _, t := range slices.Backward(stack[from:]) {
			out = append(out, t)
		}
		stack = stack[:from]
	}
	return out, nil
}

// txs groups the chain into transactions: runs of consecutive entries with
// the same tx id, as indices into c.Entries.
func (c Chain) txs() [][]int {
	var out [][]int
	for i, e := range c.Entries {
		if n := len(out); n > 0 && c.Entries[out[n-1][0]].Tx == e.Tx {
			out[n-1] = append(out[n-1], i)
			continue
		}
		out = append(out, []int{i})
	}
	return out
}

func (c Chain) nothingTo(req Request, done int) error {
	verb, unit := "undo", plural(done, "transaction", "transactions")
	if req.Redo {
		verb = "redo"
	}
	if req.Step {
		unit = plural(done, "entry", "entries")
	}
	who := " by " + req.Author
	if req.Any {
		who = ""
	}
	var e *errs.Error
	if done == 0 {
		e = errs.New(errs.NoHistory, "nothing%s to %s", who, verb)
	} else {
		e = errs.New(errs.NoHistory, "only %d %s%s to %s, not %d", done, unit, who, verb, req.N)
	}
	switch {
	case req.Redo:
		e = e.WithHint("redo only brings back what %s undid, until %s draws again", req.Author, req.Author)
	case !req.Any && c.othersCanUndo(req.Author):
		e = e.WithHint("undo --any also reverts other authors' transactions")
	}
	return e
}

func (c Chain) othersCanUndo(author string) bool {
	for _, e := range c.Entries {
		if e.Kind == "" && e.Author != author && !c.undone[e.Rev] {
			return true
		}
	}
	return false
}

type pixKey struct{ layer, frame, x, y int }

// apply reverts t on d (re-applies it for a redo) and returns what changed.
// start is d's palette before the first step of this undo or redo.
func (c Chain) apply(d *doc.Doc, t pxofile.Entry, req Request, start []doc.Entry) (ops.Change, error) {
	verb := "undoing"
	if req.Redo {
		verb = "redoing"
	}
	name := fmt.Sprintf("rev %d (%s)", t.Rev, t.Tx)
	pixels, width, err := pxofile.DecodePixelsWidth(t.Undo.Pixels)
	if err != nil {
		return ops.Change{}, err
	}
	meta, err := pxofile.DecodeMeta(t.Undo.Meta)
	if err != nil {
		return ops.Change{}, err
	}
	mismatch := func(format string, a ...any) error {
		return errs.New(errs.UndoConflict, "%s doesn't match the document: %s", name, fmt.Sprintf(format, a...)).
			WithHint("the history log is out of step with the document; --force can't override this")
	}
	if len(pixels) > 0 && width != d.W {
		return ops.Change{}, mismatch("it was written for a canvas %d wide, the document is %d wide", width, d.W)
	}
	for _, p := range pixels {
		switch {
		case p.Layer >= len(d.Layers):
			return ops.Change{}, mismatch("layer %d doesn't exist", p.Layer)
		case p.Frame >= len(d.Frames):
			return ops.Change{}, mismatch("frame %d doesn't exist", p.Frame)
		case p.Y >= d.H:
			return ops.Change{}, mismatch("pixel %d,%d is outside the %dx%d canvas", p.X, p.Y, d.W, d.H)
		}
	}
	// stuck is a conflict --force can't override.
	stuck := func(format string, a ...any) *errs.Error {
		return errs.New(errs.UndoConflict, "%s %s %s", verb, name, fmt.Sprintf(format, a...))
	}

	// want is what the document holds if nothing changed since; to is what
	// the step sets. Both are indices into the palette the entry was
	// written against: wantPal and toPal.
	want := func(p doc.PixelDelta) doc.Index { return p.After }
	to := func(p doc.PixelDelta) doc.Index { return p.Before }
	if req.Redo {
		want, to = to, want
	}
	var plan *headerPlan
	var wantPal, toPal []doc.Entry
	if meta != nil {
		w, t := meta.After, meta.Before
		if req.Redo {
			w, t = t, w
		}
		if plan, err = planHeader(w, t, d); err != nil {
			return ops.Change{}, err
		}
		wantPal, toPal = w.Palette.Entries, t.Palette.Entries
	} else {
		if wantPal, err = c.paletteAt(t.Rev, start); err != nil {
			return ops.Change{}, err
		}
		toPal = wantPal
	}
	cur := d.Palette.Entries
	wantIn := newTranslator(wantPal, cur)
	if plan != nil && plan.whole {
		wantIn = identity
	}

	if plan != nil && !plan.whole {
		if e, c, ok := plan.clash(cur); ok {
			what := "name"
			if c.Key == e.Key {
				what = "key"
			}
			return ops.Change{}, stuck("would bring back palette entry %q (key %s), but entry %q (key %s) has taken its %s since",
				e.Name, e.Key, c.Name, c.Key, what).
				WithHint("restoring it would recolor that entry's pixels, so --force can't override this")
		}
	}

	if !req.Force {
		conf := Conflict{Target: t.Rev, Pixels: [][4]int{}, Txs: []string{}}
		changed := map[pixKey]bool{}
		for _, p := range pixels {
			if w, ok := wantIn.index(want(p)); !ok || d.Pixel(p.Layer, p.Frame, p.X, p.Y) != w {
				changed[pixKey{p.Layer, p.Frame, p.X, p.Y}] = true
				if conf.Count++; conf.Count <= MaxConflictPixels {
					conf.Pixels = append(conf.Pixels, [4]int{p.Layer, p.Frame, p.X, p.Y})
				}
			}
		}
		var hit headerHit
		if plan != nil {
			if plan.whole {
				head, err := headerText(d)
				if err != nil {
					return ops.Change{}, err
				}
				w, err := headerText(plan.want)
				if err != nil {
					return ops.Change{}, err
				}
				hit.whole = head != w
			} else {
				hit.keys = plan.changedKeys(cur)
				if plan.rest {
					r, err := pxofile.HeaderRest(d)
					if err != nil {
						return ops.Change{}, err
					}
					hit.rest = r != plan.wantRest
				}
			}
		}
		conf.Meta = hit.any()
		if conf.Count > 0 || conf.Meta {
			conf.Txs = c.blame(t, req.Redo, changed, hit)
			return ops.Change{}, conflictErr(verb, name, conf, hit)
		}
	}

	var head *doc.Doc
	var headText string
	if plan != nil {
		head = d.Header()
		if headText, err = headerText(d); err != nil {
			return ops.Change{}, err
		}
	}
	var rec []doc.PixelDelta
	set := func(layer, frame, x, y int, v doc.Index) {
		if old := d.Pixel(layer, frame, x, y); old != v {
			rec = append(rec, doc.PixelDelta{Layer: layer, Frame: frame, X: x, Y: y, Before: old, After: v})
			d.SetPixel(layer, frame, x, y, v)
		}
	}
	if plan != nil && plan.whole {
		d.SetHeader(plan.to)
		for _, p := range pixels {
			set(p.Layer, p.Frame, p.X, p.Y, to(p))
		}
	} else if err := c.merge(d, plan, pixels, to, toPal, set, stuck); err != nil {
		return ops.Change{}, err
	}
	if err := d.Validate(); err != nil {
		return ops.Change{}, stuck("would leave an invalid document: %s", err).
			WithHint("a later transaction depends on it; undo that one first. --force can't override this")
	}
	ch := ops.Change{Pixels: ops.NetPixels([]ops.Change{{Pixels: rec}})}
	ch.BBox = ops.BBox(ch.Pixels)
	if plan != nil {
		after, err := headerText(d)
		if err != nil {
			return ops.Change{}, err
		}
		if after != headText {
			ch.Meta = &doc.MetaDelta{Before: head, After: d.Header()}
		}
	}
	return ch, nil
}

// merge applies a step key by key (D54): the palette entries it touched
// are merged into the document's palette, every cel, part and ramp is
// renumbered to match, the rest of the header is restored only when the
// step changed it, and the step's pixels are set, carried from toPal into
// the new palette. Pixels, ramps or parts still using an entry the merge
// drops, and pixels restored to an entry that no longer exists, are
// conflicts --force can't override.
func (c Chain) merge(d *doc.Doc, plan *headerPlan, pixels []doc.PixelDelta, to func(doc.PixelDelta) doc.Index,
	toPal []doc.Entry, set func(layer, frame, x, y int, v doc.Index), stuck func(string, ...any) *errs.Error,
) error {
	cur := d.Palette.Entries
	pal := cur
	if plan != nil {
		pal = mergePalette(cur, plan.to.Palette.Entries, plan.touched)
	}
	renum := newTranslator(cur, pal)
	inUse := func(v doc.Index, where string) error {
		return stuck("would remove palette entry %q (key %s), which %s still uses", cur[v].Name, cur[v].Key, where).
			WithHint("a later transaction depends on it; undo that one first. --force can't override this")
	}
	gone := func(e doc.Entry, where string) error {
		return stuck("would restore %s to palette entry %q (key %s), which has been removed since", where, e.Name, e.Key).
			WithHint("add the entry back with palette add first. --force can't override this")
	}

	if plan != nil {
		src := d.Header()
		if plan.rest {
			src = plan.to.Header()
		}
		from := newTranslator(src.Palette.Entries, pal)
		for _, rn := range doc.SortedKeys(src.Ramps) {
			r := src.Ramps[rn]
			for i, v := range r.Steps {
				n, ok := from.index(v)
				switch {
				case ok:
					r.Steps[i] = n
				case plan.rest:
					return gone(src.Palette.Entries[v], fmt.Sprintf("ramp %q", rn))
				default:
					return inUse(v, fmt.Sprintf("ramp %q", rn))
				}
			}
			src.Ramps[rn] = r
		}
		for _, pn := range doc.SortedKeys(src.Parts) {
			part := src.Parts[pn]
			for i, v := range part.Pix {
				n, ok := from.index(v)
				switch {
				case ok:
					part.Pix[i] = n
				case plan.rest:
					return gone(src.Palette.Entries[v], fmt.Sprintf("part %q", pn))
				default:
					return inUse(v, fmt.Sprintf("part %q", pn))
				}
			}
		}
		src.Palette.Entries = pal
		d.SetHeader(src)
	}

	// The step's own pixels, in the new palette. Keys follow links, so a
	// pixel the step sets is never renumbered as well.
	toIn := newTranslator(toPal, pal)
	own := make(map[pixKey]doc.Index, len(pixels))
	for _, p := range pixels {
		v, ok := toIn.index(to(p))
		if !ok {
			if int(to(p)) >= len(toPal) {
				return stuck("would set pixel %d,%d to index %d, past the end of its palette", p.X, p.Y, to(p)).
					WithHint("the history log is out of step with the document; --force can't override this")
			}
			return gone(toPal[to(p)], fmt.Sprintf("pixel %d,%d", p.X, p.Y))
		}
		own[pixKey{p.Layer, linkFrame(d, p.Layer, p.Frame), p.X, p.Y}] = v
	}
	if !renum.same {
		for _, ck := range d.CelKeys() {
			cel := d.Cels[ck]
			if cel.Linked {
				continue // shares the pixels of an unlinked cel, renumbered there
			}
			for i, v := range cel.Pix {
				x, y := i%d.W, i/d.W
				if _, ok := own[pixKey{ck.Layer, ck.Frame, x, y}]; ok {
					continue
				}
				n, ok := renum.index(v)
				if !ok {
					return inUse(v, fmt.Sprintf("pixel %d,%d", x, y))
				}
				set(ck.Layer, ck.Frame, x, y, n)
			}
		}
	}
	for _, p := range pixels {
		set(p.Layer, p.Frame, p.X, p.Y, own[pixKey{p.Layer, linkFrame(d, p.Layer, p.Frame), p.X, p.Y}])
	}
	return nil
}

// linkFrame is the frame whose cel holds the pixels of (layer, frame).
func linkFrame(d *doc.Doc, layer, frame int) int {
	if c := d.Cels[doc.CelKey{Layer: layer, Frame: frame}]; c != nil && c.Linked {
		return c.LinkFrame
	}
	return frame
}

// blame lists the transactions after t that changed the given pixels or
// the part of the header hit names, oldest first. For a redo the search
// starts at the latest undo of t. Entries of t's own transaction and normal
// entries that are currently undone don't count. Neither do undo and redo
// entries whose target comes after t: the target stands for their net
// effect.
func (c Chain) blame(t pxofile.Entry, redo bool, pixels map[pixKey]bool, hit headerHit) []string {
	own := map[int]bool{}
	start := -1
	for i, e := range c.Entries {
		if e.Tx == t.Tx && e.Kind == "" {
			own[e.Rev] = true
		}
		if e.Rev == t.Rev || redo && e.Kind == pxofile.KindUndo && e.Target == t.Rev {
			start = i
		}
	}
	out := []string{}
	for _, e := range c.Entries[start+1:] {
		if e.Tx == t.Tx || e.Kind != "" && (own[e.Target] || e.Target > t.Rev) || e.Kind == "" && c.undone[e.Rev] ||
			slices.Contains(out, e.Tx) {
			continue
		}
		if hit.touches(e) || len(pixels) > 0 && touchesPixels(e, pixels) {
			out = append(out, e.Tx)
		}
	}
	return out
}

func touchesPixels(e pxofile.Entry, pixels map[pixKey]bool) bool {
	ps, _ := pxofile.DecodePixels(e.Undo.Pixels)
	return slices.ContainsFunc(ps, func(p doc.PixelDelta) bool {
		return pixels[pixKey{p.Layer, p.Frame, p.X, p.Y}]
	})
}

func conflictErr(verb, name string, conf Conflict, hit headerHit) error {
	var what []string
	if conf.Count > 0 {
		what = append(what, fmt.Sprintf("%d %s", conf.Count, plural(conf.Count, "pixel", "pixels")))
	}
	switch {
	case hit.whole:
		what = append(what, "the document header (palette, ramps, ...)")
	case len(hit.keys) > 0:
		what = append(what, plural(len(hit.keys), "palette entry ", "palette entries ")+strings.Join(hit.keys, ", "))
	}
	if hit.rest {
		what = append(what, "the document header (ramps, layers, ...)")
	}
	by := "outside the history log"
	if len(conf.Txs) > 0 {
		by = "by " + strings.Join(conf.Txs, ", ")
	}
	return errs.New(errs.UndoConflict, "%s %s would overwrite later changes to %s", verb, name, strings.Join(what, " and ")).
		WithHint("they were changed %s; pass --force to %s anyway", by, strings.TrimSuffix(verb, "ing")).
		WithDetails(conf)
}

// headerText is the canonical text of d's header, as meta snapshots store it.
func headerText(d *doc.Doc) (string, error) {
	b, err := pxofile.Serialize(d.Header())
	return string(b), err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

package engine

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/history"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

func as(author string) Options { o := opts(); o.Author = author; return o }

func apply(t *testing.T, path, author string, steps ...Step) {
	t.Helper()
	if _, err := (Direct{}).Apply(path, steps, as(author)); err != nil {
		t.Fatal(err)
	}
}

func undo(t *testing.T, path, author string, req history.Request) Result {
	t.Helper()
	if req.N == 0 {
		req.N = 1
	}
	res, err := Direct{}.Undo(path, req, as(author))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func undoErr(t *testing.T, path, author string, req history.Request, code errs.Code) *errs.Error {
	t.Helper()
	if req.N == 0 {
		req.N = 1
	}
	_, err := Direct{}.Undo(path, req, as(author))
	e, ok := errors.AsType[*errs.Error](err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
	return e
}

// sameDoc compares two documents' bytes, ignoring rev.
func sameDoc(t *testing.T, got []byte, want []byte) {
	t.Helper()
	strip := func(b []byte) string {
		var out []string
		for l := range strings.Lines(string(b)) {
			if !strings.HasPrefix(l, "rev = ") {
				out = append(out, l)
			}
		}
		return strings.Join(out, "")
	}
	if a, b := strip(got), strip(want); a != b {
		t.Fatalf("document:\n%s\nwant:\n%s", a, b)
	}
}

// TestUndoRedoIdentity: undoing every transaction gives the original
// document back (palette included) apart from rev, and redoing them all
// gives the drawn document. The log of undo and redo entries is golden.
func TestUndoRedoIdentity(t *testing.T) {
	path := newFile(t)
	orig := read(t, path)
	apply(t, path, "cli", step(t, "", "paint o 0,0 1,0 2,0 7,7"))
	apply(t, path, "cli", step(t, "blue", "paint #3b5dc9 0,0 4,4"), step(t, "", "paint #ffffff 5,5 0,0"))
	apply(t, path, "cli", step(t, "", "paint . 7,7"))
	drawn := read(t, path)

	res := undo(t, path, "cli", history.Request{N: 3})
	if got, want := asJSON(t, res), `{"ok":true,"rev":8,"changed":5,"bbox":[0,0,6,6],"undone":[{"rev":4,"tx":"t_0004"},{"rev":3,"tx":"t_0002"},{"rev":2,"tx":"t_0002"},{"rev":1,"tx":"t_0001"}],"warnings":[]}`; got != want {
		t.Fatalf("undo = %s\nwant   %s", got, want)
	}
	sameDoc(t, read(t, path), orig)
	_ = undoErr(t, path, "cli", history.Request{}, errs.NoHistory)

	res = undo(t, path, "cli", history.Request{Redo: true, N: 3})
	if got, want := asJSON(t, res), `{"ok":true,"rev":12,"changed":5,"bbox":[0,0,6,6],"redone":[{"rev":1,"tx":"t_0001"},{"rev":2,"tx":"t_0002"},{"rev":3,"tx":"t_0002"},{"rev":4,"tx":"t_0004"}],"warnings":[]}`; got != want {
		t.Fatalf("redo = %s\nwant   %s", got, want)
	}
	sameDoc(t, read(t, path), drawn)
	_ = undoErr(t, path, "cli", history.Request{Redo: true}, errs.NoHistory)
	golden.Check(t, "testdata/undo.golden.pxo.log", read(t, pxofile.LogPath(path)))
}

// TestUndoStep: --step reverts one entry of a multi-step transaction, and a
// later undo reverts the rest of it.
func TestUndoStep(t *testing.T) {
	path := newFile(t)
	orig := read(t, path)
	apply(t, path, "cli", step(t, "a", "paint o 0,0"), step(t, "b", "paint o 1,0"), step(t, "c", "paint o 2,0"))
	if res := undo(t, path, "cli", history.Request{Step: true}); res.Changed != 1 || res.Undone[0].Rev != 3 {
		t.Fatalf("undo --step = %s", asJSON(t, res))
	}
	if res := undo(t, path, "cli", history.Request{}); len(res.Undone) != 2 || res.Undone[0].Rev != 2 {
		t.Fatalf("undo = %s", asJSON(t, res))
	}
	sameDoc(t, read(t, path), orig)
	// Redo --step brings back the earliest undone entry first.
	if res := undo(t, path, "cli", history.Request{Redo: true, Step: true}); res.Redone[0].Rev != 1 {
		t.Fatalf("redo --step = %s", asJSON(t, res))
	}
}

// TestUndoPerAuthor: undo reverts the author's own latest transaction, even
// when another author wrote after it; --any reverts anyone's.
func TestUndoPerAuthor(t *testing.T) {
	path := newFile(t)
	apply(t, path, "a", step(t, "", "paint o 0,0"))
	apply(t, path, "b", step(t, "", "paint o 5,5"))
	if res := undo(t, path, "a", history.Request{}); res.Undone[0].Tx != "t_0001" {
		t.Fatalf("a's undo = %s", asJSON(t, res))
	}
	e := undoErr(t, path, "a", history.Request{}, errs.NoHistory)
	if e.Message != "nothing by a to undo" || !strings.Contains(e.Hint, "--any") {
		t.Fatalf("no_history = %q, hint %q", e.Message, e.Hint)
	}
	if res := undo(t, path, "c", history.Request{Any: true}); res.Undone[0].Tx != "t_0002" {
		t.Fatalf("undo --any = %s", asJSON(t, res))
	}
	// b's transaction was undone by c, so only c can redo it.
	_ = undoErr(t, path, "b", history.Request{Redo: true}, errs.NoHistory)
	undo(t, path, "c", history.Request{Redo: true})
	e = undoErr(t, path, "a", history.Request{N: 2, Redo: true}, errs.NoHistory)
	if e.Message != "only 1 transaction by a to redo, not 2" {
		t.Fatalf("message %q", e.Message)
	}
}

// TestRedoClearedByNewWork: drawing again after an undo drops the redo.
func TestRedoClearedByNewWork(t *testing.T) {
	path := newFile(t)
	apply(t, path, "cli", step(t, "", "paint o 0,0"))
	undo(t, path, "cli", history.Request{})
	apply(t, path, "other", step(t, "", "paint o 6,6"))
	undo(t, path, "cli", history.Request{Redo: true}) // another author's work doesn't clear it
	undo(t, path, "cli", history.Request{})
	apply(t, path, "cli", step(t, "", "paint o 3,3"))
	_ = undoErr(t, path, "cli", history.Request{Redo: true}, errs.NoHistory)
}

// TestUndoConflict: a later transaction by another author on the same pixel
// makes undo fail with details, and --force reverts anyway.
func TestUndoConflict(t *testing.T) {
	path := newFile(t)
	apply(t, path, "a", step(t, "", "paint o 1,1 2,2"))
	apply(t, path, "b", step(t, "", "paint #ff0000 1,1"))
	e := undoErr(t, path, "a", history.Request{}, errs.UndoConflict)
	if got, want := asJSON(t, e), `{"code":"undo_conflict","message":"undoing rev 1 (t_0001) would overwrite later changes to 1 pixel","hint":"they were changed by t_0002; pass --force to undo anyway","details":{"target":1,"count":1,"pixels":[[1,1]],"txs":["t_0002"]}}`; got != want {
		t.Fatalf("error = %s\nwant    %s", got, want)
	}
	if errs.ExitCode(e.Code) != errs.ExitConflict {
		t.Fatal("undo_conflict must exit 3")
	}
	res := undo(t, path, "a", history.Request{Force: true})
	if got, want := asJSON(t, res), `{"ok":true,"rev":3,"changed":2,"bbox":[1,1,2,2],"undone":[{"rev":1,"tx":"t_0001"}],"warnings":[]}`; got != want {
		t.Fatalf("forced undo = %s", got)
	}
	d, _ := Direct{}.Load(path)
	if d.Pixel(0, 0, 1, 1) != doc.Transparent || d.Palette.Len() != 3 {
		t.Fatal("forced undo should clear 1,1 and keep b's palette entry")
	}
}

// TestUndoHeaderConflict: a header changed since (another auto entry) is a
// conflict for an entry that changed the header.
func TestUndoHeaderConflict(t *testing.T) {
	path := newFile(t)
	apply(t, path, "a", step(t, "", "paint #ff0000 1,1"))
	apply(t, path, "b", step(t, "", "paint #00ff00 5,5"))
	e := undoErr(t, path, "a", history.Request{}, errs.UndoConflict)
	if d := e.Details.(history.Conflict); !d.Meta || d.Count != 0 || !reflect.DeepEqual(d.Txs, []string{"t_0002"}) {
		t.Fatalf("details = %s", asJSON(t, d))
	}
	// Forcing would drop b's entry while b's pixel still uses it.
	e = undoErr(t, path, "a", history.Request{Force: true}, errs.UndoConflict)
	if !strings.Contains(e.Message, "invalid document") {
		t.Fatalf("message %q", e.Message)
	}
}

// TestUndoEntryInUse: b paints with the entry a's transaction added. Neither
// the pixels nor the header changed since, but undoing a would leave b's
// pixel without a palette entry, so it is refused even with --force.
func TestUndoEntryInUse(t *testing.T) {
	path := newFile(t)
	apply(t, path, "a", step(t, "", "paint #ff0000 1,1"))
	apply(t, path, "b", step(t, "", "paint a 5,5"))
	before := read(t, path)
	for _, force := range []bool{false, true} {
		e := undoErr(t, path, "a", history.Request{Force: force}, errs.UndoConflict)
		if !strings.Contains(e.Message, "would leave an invalid document") || !strings.Contains(e.Hint, "can't override") {
			t.Fatalf("force=%v: %q / %q", force, e.Message, e.Hint)
		}
	}
	if !bytes.Equal(read(t, path), before) {
		t.Fatal("a failed undo wrote the document")
	}
}

// TestUndoHandEdit: a pixel changed outside pixopher is a conflict that no
// transaction is blamed for.
func TestUndoHandEdit(t *testing.T) {
	path := newFile(t)
	apply(t, path, "cli", step(t, "", "paint o 1,1"))
	d, _ := Direct{}.Load(path)
	d.SetPixel(0, 0, 1, 1, doc.Transparent)
	d.SetPixel(0, 0, 2, 1, 1)
	if err := os.WriteFile(path, serialize(t, d), 0o644); err != nil {
		t.Fatal(err)
	}
	e := undoErr(t, path, "cli", history.Request{}, errs.UndoConflict)
	if !strings.Contains(e.Hint, "outside the history log") || len(e.Details.(history.Conflict).Txs) != 0 {
		t.Fatalf("hint %q, details %s", e.Hint, asJSON(t, e.Details))
	}
}

// TestLogAhead: a document rolled back (as by git checkout) while its log
// was not. Entries above its rev are ignored with a warning, the next
// transaction reuses their revs and tx ids, and the abandoned entries never
// come back.
func TestLogAhead(t *testing.T) {
	path := newFile(t)
	apply(t, path, "cli", step(t, "", "paint o 0,0"))
	saved := read(t, path)
	apply(t, path, "cli", step(t, "", "paint o 1,0"))
	apply(t, path, "cli", step(t, "", "paint o 2,0"))
	if err := os.WriteFile(path, saved, 0o644); err != nil {
		t.Fatal(err)
	}
	_, c, err := Direct{}.History(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 1 || len(c.Warnings) != 1 ||
		c.Warnings[0] != "the history log has 2 entries above the document's rev 1; they are ignored" {
		t.Fatalf("chain %d entries, warnings %q", len(c.Entries), c.Warnings)
	}
	apply(t, path, "cli", step(t, "", "paint o 5,5")) // rev 2 again, tx t_0002 again
	res := undo(t, path, "cli", history.Request{N: 2})
	if got := asJSON(t, res.Undone); got != `[{"rev":2,"tx":"t_0002"},{"rev":1,"tx":"t_0001"}]` {
		t.Fatalf("undone %s", got)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings %q", res.Warnings)
	}
	d, _ := Direct{}.Load(path)
	if len(d.Cels) != 0 {
		t.Fatal("canvas should be empty")
	}
}

// TestLogBehind: the document was written but its log append was lost.
// Undo still works on the logged entries and warns about the gap.
func TestLogBehind(t *testing.T) {
	path := newFile(t)
	apply(t, path, "cli", step(t, "", "paint o 0,0"))
	logData := read(t, pxofile.LogPath(path))
	apply(t, path, "cli", step(t, "", "paint o 1,0"))
	if err := os.WriteFile(pxofile.LogPath(path), logData, 0o644); err != nil {
		t.Fatal(err)
	}
	res := undo(t, path, "cli", history.Request{})
	if res.Undone[0].Rev != 1 || len(res.Warnings) != 1 ||
		res.Warnings[0] != "the document is at rev 2 but its history ends at rev 1; later changes can't be undone" {
		t.Fatalf("undo = %s", asJSON(t, res))
	}
}

// TestUndoMismatch: an entry written for another canvas is refused, even
// with --force.
func TestUndoMismatch(t *testing.T) {
	path := newFile(t)
	e := pxofile.Entry{
		Rev: 1, Tx: "t_0001", TS: "2026-11-24T09:00:00Z", Author: "cli", Ops: []string{"x"},
		Undo: pxofile.Undo{Pixels: pxofile.EncodePixels([]doc.PixelDelta{{X: 1, Y: 1, Before: 0, After: 1}}, 16)},
	}
	if err := pxofile.AppendLog(pxofile.LogPath(path), []pxofile.Entry{e}); err != nil {
		t.Fatal(err)
	}
	d, _ := Direct{}.Load(path)
	d.Rev = 1
	if err := os.WriteFile(path, serialize(t, d), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		e := undoErr(t, path, "cli", history.Request{Force: force}, errs.UndoConflict)
		if e.Message != "rev 1 (t_0001) doesn't match the document: it was written for a canvas 16 wide, the document is 8 wide" {
			t.Fatalf("message %q", e.Message)
		}
	}
}

// TestUndoDryRunAndExpectRev: a dry run writes nothing; --expect-rev applies.
func TestUndoDryRunAndExpectRev(t *testing.T) {
	path := newFile(t)
	apply(t, path, "cli", step(t, "", "paint o 0,0"))
	before, beforeLog := read(t, path), read(t, pxofile.LogPath(path))
	o := opts()
	o.DryRun = true
	res, err := Direct{}.Undo(path, history.Request{N: 1}, o)
	if err != nil || !res.DryRun || res.Rev != 2 {
		t.Fatalf("dry run = %s, %v", asJSON(t, res), err)
	}
	if !bytes.Equal(read(t, path), before) || !bytes.Equal(read(t, pxofile.LogPath(path)), beforeLog) {
		t.Fatal("dry run wrote something")
	}
	o = opts()
	rev := 0
	o.ExpectRev = &rev
	if _, err := (Direct{}).Undo(path, history.Request{N: 1}, o); errs.CodeOf(err) != errs.RevConflict {
		t.Fatalf("err = %v", err)
	}
}

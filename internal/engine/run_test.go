package engine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// line gives c the script line n.
func line(n int, c ops.Call) ops.Call { c.LineNo = n; return c }

func set(t *testing.T, n int, tokens ...string) ops.Call {
	t.Helper()
	c, err := ops.ParseSet(tokens)
	if err != nil {
		t.Fatal(err)
	}
	return line(n, c)
}

// TestScriptErrorNamesLine: a failing call from a script line fails as a
// script_error naming it, and nothing is written; a call from the command
// line fails with its own error.
func TestScriptErrorNamesLine(t *testing.T) {
	path := newFile(t)
	before := read(t, path)
	steps := []Step{{Calls: []ops.Call{line(1, paint(t, "paint o 1,1")), line(7, paint(t, "paint o 9,9"))}}}
	_, err := Direct{}.Apply(path, steps, opts())
	e, ok := errors.AsType[*errs.Error](err)
	if !ok || e.Code != errs.ScriptError || e.Line != 7 || e.Cause != errs.OutOfBounds ||
		e.Message != "line 7: out_of_bounds: 9,9 is outside the canvas" || e.Arg != "at" {
		t.Fatalf("got %#v", err)
	}
	if string(read(t, path)) != string(before) || exists(pxofile.LogPath(path)) {
		t.Fatal("a failed script wrote something")
	}
	_, err = Direct{}.Apply(path, []Step{step(t, "", "paint o 9,9")}, opts())
	if errs.CodeOf(err) != errs.OutOfBounds {
		t.Fatalf("got %v, want out_of_bounds", err)
	}
}

// TestContinueOnError: a failing call is skipped and what it wrote before
// failing, pixels and palette, is undone; the warnings name the lines in
// order. The result is the same document as a run without those calls.
func TestContinueOnError(t *testing.T) {
	path := newFile(t)
	opt := opts()
	opt.ContinueOnError = true
	steps := []Step{
		{Msg: "a", Calls: []ops.Call{
			line(1, paint(t, "paint o 1,1")),
			// Adds a palette entry, overwrites 1,1 and writes 2,2, then fails.
			line(2, paint(t, "paint #ff0000 1,1 2,2 9,9")),
			line(3, paint(t, "paint o 3,3")),
		}},
		{Msg: "b", Calls: []ops.Call{line(5, paint(t, "paint #00ff00 4,4 -1,0"))}},
	}
	res, err := Direct{}.Apply(path, steps, opt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"line 2 skipped: out_of_bounds: 9,9 is outside the canvas",
		"line 5 skipped: out_of_bounds: -1,0 is outside the canvas",
	}
	if !reflect.DeepEqual(res.Warnings, want) || res.Rev != 1 || res.Changed != 2 {
		t.Fatalf("got %+v", res)
	}

	clean := newFile(t)
	if _, err := (Direct{}).Apply(clean, []Step{{Msg: "a", Calls: []ops.Call{paint(t, "paint o 1,1"), paint(t, "paint o 3,3")}}}, opts()); err != nil {
		t.Fatal(err)
	}
	if string(read(t, path)) != string(read(t, clean)) {
		t.Fatalf("skipped calls left something behind:\n%s", read(t, path))
	}
	log, err := pxofile.ReadLog(pxofile.LogPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || !reflect.DeepEqual(log[0].Ops, []string{"paint o 1,1", "paint o 3,3"}) {
		t.Fatalf("log %+v", log)
	}
}

// twoLayers writes an 8x8 document with the layers main and top.
func twoLayers(t *testing.T) string {
	t.Helper()
	path := newFile(t)
	d, err := pxofile.Parse(read(t, path))
	if err != nil {
		t.Fatal(err)
	}
	d.Layers = append(d.Layers, doc.NewLayer("top"))
	data, err := pxofile.Serialize(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSetCarriesAcrossSteps: @set holds until the next one, across steps,
// and an entry that starts on another target than the default names it.
func TestSetCarriesAcrossSteps(t *testing.T) {
	path := twoLayers(t)
	steps := []Step{
		{Msg: "a", Calls: []ops.Call{line(1, paint(t, "paint o 0,0")), set(t, 2, "layer=top"), line(3, paint(t, "paint o 1,1"))}},
		{Msg: "b", Calls: []ops.Call{line(5, paint(t, "paint o 2,2"))}},
		{Msg: "c", Calls: []ops.Call{set(t, 7, "layer=0", "frame=0"), line(8, paint(t, "paint o 3,3"))}},
	}
	if _, err := (Direct{}).Apply(path, steps, opts()); err != nil {
		t.Fatal(err)
	}
	d, err := Direct{}.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ink := doc.Index(d.Palette.Len() - 1)
	for _, p := range []struct{ layer, x int }{{0, 0}, {1, 1}, {1, 2}, {0, 3}} {
		if d.Pixel(p.layer, 0, p.x, p.x) != ink {
			t.Errorf("pixel %d,%d is not on layer %d", p.x, p.x, p.layer)
		}
	}
	if d.Pixel(0, 0, 2, 2) != doc.Transparent {
		t.Error("step b drew on layer 0")
	}
	log, err := pxofile.ReadLog(pxofile.LogPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range log {
		got = append(got, strings.Join(e.Ops, "; "))
	}
	want := []string{
		"paint o 0,0; @set layer=top; paint o 1,1",
		"@set layer=top frame=0; paint o 2,2",
		"@set layer=top frame=0; @set layer=0 frame=0; paint o 3,3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("log ops\n%q\nwant\n%q", got, want)
	}
}

func TestSetErrors(t *testing.T) {
	path := newFile(t)
	for _, tt := range []struct {
		tokens []string
		code   errs.Code
		msg    string
	}{
		{[]string{"layer=top"}, errs.UnknownLayer, `line 4: unknown_layer: there is no layer "top"`},
		{[]string{"layer=1"}, errs.UnknownLayer, "line 4: unknown_layer: there is no layer 1"},
		{[]string{"frame=2"}, errs.UnknownFrame, "line 4: unknown_frame: there is no frame 2"},
		{[]string{"frame=tag:idle"}, errs.UnknownFrame, `line 4: unknown_frame: there is no tag "idle"`},
	} {
		_, err := Direct{}.Apply(path, []Step{{Calls: []ops.Call{set(t, 4, tt.tokens...)}}}, opts())
		if e, ok := errors.AsType[*errs.Error](err); !ok || e.Cause != tt.code || e.Message != tt.msg {
			t.Errorf("@set %s: got %#v", tt.tokens, err)
		}
	}
}

// TestOpEvents: with OnOp set, every applied op (not @set, not a skipped
// one) gives one event with the pixels it wrote, and Apply waits Pace
// after each. The file is still written once, at the end.
func TestOpEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.pxo")
	if err := os.WriteFile(path, read(t, newFile(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []OpEvent
	opt := opts()
	opt.ContinueOnError = true
	opt.Pace = 5 * time.Millisecond
	opt.OnOp = func(e OpEvent) {
		if exists(pxofile.LogPath(path)) {
			t.Error("the log was written before the last op")
		}
		events = append(events, e)
	}
	steps := []Step{
		{Calls: []ops.Call{line(1, paint(t, "paint o 1,1 2,1")), set(t, 2, "frame=0"), line(3, paint(t, "paint o 9,9")), line(4, paint(t, "paint o 1,1"))}},
		{Calls: []ops.Call{line(6, paint(t, "paint o 3,3"))}},
	}
	start := time.Now()
	if _, err := (Direct{}).Apply(path, steps, opt); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 3*opt.Pace {
		t.Errorf("took %s, want at least 3 paces", took)
	}
	var got []string
	for _, e := range events {
		var px []string
		for _, p := range e.Pixels {
			px = append(px, asJSON(t, []int{p.X, p.Y, int(p.Before), int(p.After)}))
		}
		got = append(got, asJSON(t, e.Rev)+" "+e.Op+" "+strings.Join(px, ""))
	}
	want := []string{"1 paint o 1,1 2,1 [1,1,0,1][2,1,0,1]", "1 paint o 1,1 [1,1,0,1]", "2 paint o 3,3 [3,3,0,1]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events\n%q\nwant\n%q", got, want)
	}
}

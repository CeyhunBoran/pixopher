package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// paintOp is a test-only op: `paint <color> <x,y>...`. A raw hex color that
// is not in the palette adds an auto entry, so it also exercises meta
// changes. It lives on a private registry and never reaches ops.Default.
type paintOp struct{}

func (paintOp) Spec() ops.Spec {
	return ops.Spec{
		Name: "paint", Group: "test", Summary: "set pixels to a color", Mutates: true,
		Args: []ops.ArgSpec{
			{Name: "color", Kind: ops.KindColor, Required: true},
			{Name: "at", Kind: ops.KindPoint, Required: true, Repeat: true},
		},
	}
}

func (paintOp) Apply(ctx *ops.Ctx, args ops.Args) error {
	r, err := color.Resolve(ctx.Doc, args.String("color"))
	if err != nil {
		return err
	}
	if !r.InPalette {
		e, err := ctx.Doc.Palette.AutoEntry(r.Color, ctx.Doc.KeyWidth)
		if err != nil {
			return err
		}
		r.Index = doc.Index(ctx.Doc.Palette.Len())
		ctx.Doc.Palette.Entries = append(ctx.Doc.Palette.Entries, e)
	}
	for _, p := range args["at"].([]doc.Point) {
		if !ctx.In(p.X, p.Y) {
			return errs.New(errs.OutOfBounds, "%d,%d is outside the canvas", p.X, p.Y).WithArg("at")
		}
		ctx.Set(p.X, p.Y, r.Index)
	}
	return nil
}

var testOps = func() *ops.Registry {
	r := ops.NewRegistry()
	r.Register(paintOp{})
	return r
}()

// paint builds a call from a script-like line: "paint o 1,1 2,2".
func paint(t testing.TB, line string) ops.Call {
	t.Helper()
	f := strings.Fields(line)
	var pts []doc.Point
	for _, s := range f[2:] {
		var p doc.Point
		if _, err := fmt.Sscanf(s, "%d,%d", &p.X, &p.Y); err != nil {
			t.Fatalf("bad point %q", s)
		}
		pts = append(pts, p)
	}
	op, _ := testOps.Lookup(f[0])
	return ops.Call{Op: op.(ops.Mutator), Args: ops.Args{"color": f[1], "at": pts}, Line: line}
}

func step(t testing.TB, msg string, lines ...string) Step {
	s := Step{Msg: msg}
	for _, l := range lines {
		s.Calls = append(s.Calls, paint(t, l))
	}
	return s
}

var clock = func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.FixedZone("x", 3600)) }

func opts() Options { return Options{Now: clock, LockTimeout: 50 * time.Millisecond} }

// newFile writes an 8x8 document with an "ink" entry (key o) to a temp dir.
func newFile(t testing.TB) string {
	t.Helper()
	d, err := doc.New(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "o", Name: "ink", Color: doc.RGBA{R: 0x1a, G: 0x1c, B: 0x2c, A: 255}})
	data, err := pxofile.Serialize(d)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sprite.pxo")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func asJSON(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestApplyGolden: a two-step transaction gives fixed document and log bytes.
func TestApplyGolden(t *testing.T) {
	path := newFile(t)
	res, err := Direct{}.Apply(path, []Step{
		step(t, "outline", "paint o 1,1 2,1 3,1", "paint o 1,2"),
		step(t, "", "paint #ff0000 2,2 3,2"),
	}, Options{Now: clock, Author: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := asJSON(t, res), `{"ok":true,"rev":2,"changed":6,"bbox":[1,1,3,2],"warnings":[]}`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
	golden.Check(t, "testdata/apply.golden.pxo", read(t, path))
	golden.Check(t, "testdata/apply.golden.pxo.log", read(t, pxofile.LogPath(path)))
}

// TestUndoFromLog: the log alone is enough to revert a transaction. Reading
// the entries back and reverting them, newest first, gives the original
// document (apart from rev, which only moves forward); reapplying them gives
// the drawn document again.
func TestUndoFromLog(t *testing.T) {
	path := newFile(t)
	orig, _ := Direct{}.Load(path)
	_, err := Direct{}.Apply(path, []Step{
		step(t, "", "paint o 0,0 1,0 2,0 7,7"),
		step(t, "", "paint #3b5dc9 0,0 4,4", "paint #ffffff 5,5"),
		step(t, "", "paint . 7,7"),
	}, opts())
	if err != nil {
		t.Fatal(err)
	}
	drawn, _ := Direct{}.Load(path)
	entries, err := pxofile.ReadLog(pxofile.LogPath(path))
	if err != nil || len(entries) != 3 {
		t.Fatalf("log: %d entries, %v", len(entries), err)
	}
	changes := make([]ops.Change, len(entries))
	for i, e := range entries {
		px, err := pxofile.DecodePixels(e.Undo.Pixels)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := pxofile.DecodeMeta(e.Undo.Meta)
		if err != nil {
			t.Fatal(err)
		}
		changes[i] = ops.Change{Pixels: px, Meta: meta}
	}
	if changes[1].Meta == nil || changes[0].Meta != nil {
		t.Fatal("only the step that added colors should carry a meta delta")
	}
	d := drawn.Clone()
	for _, change := range slices.Backward(changes) {
		ops.Revert(d, change)
	}
	d.Rev = orig.Rev
	if a, b := serialize(t, d), serialize(t, orig); !bytes.Equal(a, b) {
		t.Fatalf("revert from log:\n%s\nwant:\n%s", a, b)
	}
	for _, ch := range changes {
		ops.Reapply(d, ch)
	}
	d.Rev = drawn.Rev
	if a, b := serialize(t, d), serialize(t, drawn); !bytes.Equal(a, b) {
		t.Fatalf("reapply from log:\n%s\nwant:\n%s", a, b)
	}
}

func serialize(t testing.TB, d *doc.Doc) []byte {
	t.Helper()
	b, err := pxofile.Serialize(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRevAndTx: every changing step bumps rev and writes an entry; entries of
// one transaction share the tx id of its first rev; empty steps are skipped.
func TestRevAndTx(t *testing.T) {
	path := newFile(t)
	if _, err := (Direct{}).Apply(path, []Step{step(t, "", "paint o 0,0")}, opts()); err != nil {
		t.Fatal(err)
	}
	res, err := Direct{}.Apply(path, []Step{
		step(t, "a", "paint o 1,0"),
		step(t, "noop", "paint o 1,0"),
		step(t, "b", "paint o 2,0"),
	}, opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Rev != 3 || res.Changed != 2 {
		t.Fatalf("rev %d changed %d, want 3 and 2", res.Rev, res.Changed)
	}
	es, _ := pxofile.ReadLog(pxofile.LogPath(path))
	var got []string
	for _, e := range es {
		got = append(got, fmt.Sprintf("%d %s %s", e.Rev, e.Tx, e.Msg))
	}
	if want := []string{"1 t_0001 ", "2 t_0002 a", "3 t_0002 b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %q, want %q", got, want)
	}
	if d, _ := (Direct{}).Load(path); d.Rev != 3 {
		t.Fatalf("file rev %d", d.Rev)
	}
}

// TestNoOpWritesNothing: a transaction that changes nothing keeps the file,
// writes no log and does not bump rev.
func TestNoOpWritesNothing(t *testing.T) {
	path := newFile(t)
	before := read(t, path)
	res, err := Direct{}.Apply(path, []Step{step(t, "", "paint . 3,3")}, opts())
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, res); got != `{"ok":true,"rev":0,"changed":0,"warnings":[]}` {
		t.Fatalf("result = %s", got)
	}
	if !bytes.Equal(read(t, path), before) || exists(pxofile.LogPath(path)) {
		t.Fatal("no-op transaction wrote something")
	}
}

func TestExpectRev(t *testing.T) {
	path := newFile(t)
	o := opts()
	zero := 0
	o.ExpectRev = &zero
	if _, err := (Direct{}).Apply(path, []Step{step(t, "", "paint o 0,0")}, o); err != nil {
		t.Fatalf("matching --expect-rev: %v", err)
	}
	before := read(t, path)
	_, err := Direct{}.Apply(path, []Step{step(t, "", "paint o 1,1")}, o) // doc is at rev 1 now
	e, ok := errors.AsType[*errs.Error](err)
	if !ok || e.Code != errs.RevConflict || errs.ExitCode(e.Code) != errs.ExitConflict || e.Arg != "expect-rev" {
		t.Fatalf("stale --expect-rev: %v", err)
	}
	if !strings.Contains(e.Hint, "--expect-rev 1") {
		t.Fatalf("hint %q should name the current rev", e.Hint)
	}
	if !bytes.Equal(read(t, path), before) {
		t.Fatal("rev_conflict wrote the file")
	}
	if es, _ := pxofile.ReadLog(pxofile.LogPath(path)); len(es) != 1 {
		t.Fatalf("rev_conflict appended to the log: %d entries", len(es))
	}
}

// TestDryRun: the result matches a real run, nothing is written, no lock is
// taken, and --expect-rev is still checked.
func TestDryRun(t *testing.T) {
	path := newFile(t)
	before := read(t, path)
	o := opts()
	o.DryRun = true
	steps := []Step{step(t, "", "paint #ff0000 1,1 2,2")}
	res, err := Direct{}.Apply(path, steps, o)
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, res); got != `{"ok":true,"rev":1,"changed":2,"bbox":[1,1,2,2],"warnings":[],"dry_run":true}` {
		t.Fatalf("dry run result = %s", got)
	}
	if !bytes.Equal(read(t, path), before) || exists(pxofile.LogPath(path)) || exists(LockPath(path)) {
		t.Fatal("dry run wrote something")
	}
	stale := 5
	o.ExpectRev = &stale
	if _, err := (Direct{}).Apply(path, steps, o); errs.CodeOf(err) != errs.RevConflict {
		t.Fatalf("dry run with stale --expect-rev: %v", err)
	}
	wet, err := Direct{}.Apply(path, steps, opts())
	if err != nil {
		t.Fatal(err)
	}
	wet.DryRun = true
	if !reflect.DeepEqual(wet, res) {
		t.Fatalf("dry run %+v differs from real run %+v", res, wet)
	}
}

// TestFailedCallWritesNothing: a transaction is atomic. If a later call
// fails, earlier steps are not written either.
func TestFailedCallWritesNothing(t *testing.T) {
	path := newFile(t)
	before := read(t, path)
	_, err := Direct{}.Apply(path, []Step{
		step(t, "", "paint o 0,0"),
		step(t, "", "paint o 1,1 9,9"),
	}, opts())
	if errs.CodeOf(err) != errs.OutOfBounds {
		t.Fatalf("err = %v, want out_of_bounds", err)
	}
	if !bytes.Equal(read(t, path), before) || exists(pxofile.LogPath(path)) {
		t.Fatal("failed transaction wrote something")
	}
}

func TestMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.pxo")
	_, err := Direct{}.Apply(path, nil, opts())
	if errs.CodeOf(err) != errs.NoFile {
		t.Fatalf("err = %v, want no_file", err)
	}
	if exists(LockPath(path)) {
		t.Fatal("a missing document should not get a lock file")
	}
}

// TestLocked: a writer that cannot get the lock in time fails with `locked`
// (exit 3) and writes nothing.
func TestLocked(t *testing.T) {
	path := newFile(t)
	l, err := acquire(LockPath(path), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	before := read(t, path)
	start := time.Now()
	_, err = Direct{}.Apply(path, []Step{step(t, "", "paint o 0,0")}, opts())
	if errs.CodeOf(err) != errs.Locked || errs.ExitCode(errs.Locked) != errs.ExitConflict {
		t.Fatalf("err = %v, want locked", err)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("did not wait for the lock timeout")
	}
	if !bytes.Equal(read(t, path), before) {
		t.Fatal("locked writer wrote the file")
	}
	if err := l.release(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Direct{}).Apply(path, []Step{step(t, "", "paint o 0,0")}, opts()); err != nil {
		t.Fatalf("after release: %v", err)
	}
	if !exists(LockPath(path)) {
		t.Fatal("the lock file must stay on disk")
	}
}

// TestLoadIgnoresLock: readers such as observation ops (D38) load without
// the lock, so a writer holding it never blocks them.
func TestLoadIgnoresLock(t *testing.T) {
	path := newFile(t)
	l, err := acquire(LockPath(path), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.release() }()
	start := time.Now()
	if _, err := (Direct{}).Load(path); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) >= 50*time.Millisecond {
		t.Fatal("Load waited for the lock")
	}
}

// TestConcurrentWriters: the lock serializes writers, so no write is lost
// and revs are 1..n with one entry each.
func TestConcurrentWriters(t *testing.T) {
	path := newFile(t)
	const n = 16
	var wg sync.WaitGroup
	errc := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			o := opts()
			o.LockTimeout = 10 * time.Second
			_, err := Direct{}.Apply(path, []Step{step(t, "", fmt.Sprintf("paint o %d,%d", i%8, i/8))}, o)
			errc <- err
		})
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
	d, _ := Direct{}.Load(path)
	if d.Rev != n {
		t.Fatalf("rev %d, want %d", d.Rev, n)
	}
	for i := range n {
		if d.Pixel(0, 0, i%8, i/8) != 1 {
			t.Fatalf("write %d was lost", i)
		}
	}
	es, _ := pxofile.ReadLog(pxofile.LogPath(path))
	for i, e := range es {
		if e.Rev != i+1 {
			t.Fatalf("entry %d has rev %d", i, e.Rev)
		}
	}
	if len(es) != n {
		t.Fatalf("%d entries, want %d", len(es), n)
	}
}

// TestAtomicWrite: no temp files are left behind, and the file keeps its
// permissions.
func TestAtomicWrite(t *testing.T) {
	path := newFile(t)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (Direct{}).Apply(path, []Step{step(t, "", "paint o 0,0")}, opts()); err != nil {
		t.Fatal(err)
	}
	names, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*"))
	hidden, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*"))
	var got []string
	for _, n := range append(names, hidden...) {
		got = append(got, filepath.Base(n))
	}
	if want := []string{"sprite.pxo", "sprite.pxo.lock", "sprite.pxo.log"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(path)
		if st.Mode().Perm() != 0o640 {
			t.Fatalf("mode = %v, want 0640", st.Mode().Perm())
		}
	}
}

// BenchmarkApply64 is one single-step command on a 64x64 document: lock,
// parse, apply, serialize, write, log. The budget is 15 ms (docs/roadmap.md).
func BenchmarkApply64(b *testing.B) {
	d, _ := doc.New(64, 64)
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "o", Name: "ink", Color: doc.RGBA{A: 255}})
	for y := range 64 {
		for x := range 64 {
			if (x+y)%3 == 0 {
				d.SetPixel(0, 0, x, y, 1)
			}
		}
	}
	path := filepath.Join(b.TempDir(), "big.pxo")
	data, _ := pxofile.Serialize(d)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		b.Fatal(err)
	}
	var pts []doc.Point
	for x := range 16 {
		pts = append(pts, doc.Point{X: x, Y: 10})
	}
	op, _ := testOps.Lookup("paint")
	colors := []string{"o", "."}
	b.ResetTimer()
	for i := range b.N {
		call := ops.Call{Op: op.(ops.Mutator), Args: ops.Args{"color": colors[i%2], "at": pts}, Line: "paint"}
		if _, err := (Direct{}).Apply(path, []Step{{Calls: []ops.Call{call}}}, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// newCall builds a call of the built-in new op.
func newCall(t testing.TB, tokens ...string) ops.Call {
	t.Helper()
	op, ok := ops.Default.Lookup("new")
	if !ok {
		t.Fatal("no new op")
	}
	args, err := ops.Parse(op.Spec().Args, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return ops.Call{Op: op.(ops.Mutator), Args: args, Line: ops.Format(op.Spec(), args)}
}

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hero.pxo")
	var b Direct

	// A dry run reports and writes nothing, not even the lock file.
	res, err := b.Create(path, newCall(t, path, "--size", "4x2", "--bg", "#102030"), Options{Now: clock, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, res); got != `{"ok":true,"rev":0,"changed":8,"bbox":[0,0,4,2],"warnings":[],"dry_run":true}` {
		t.Fatalf("dry run: %s", got)
	}
	if exists(path) || exists(LockPath(path)) {
		t.Fatal("dry run wrote a file")
	}

	res, err = b.Create(path, newCall(t, path, "--size", "4x2", "--bg", "#102030"), opts())
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, res); got != `{"ok":true,"rev":0,"changed":8,"bbox":[0,0,4,2],"warnings":[]}` {
		t.Fatalf("create: %s", got)
	}
	want := "pixopher = 1\nsize = [4, 2]\nrev = 0\nkey_width = 1\n\n[meta]\ncreated = 2026-11-24T09:00:00Z\n\n" +
		"[palette]\n\".\" = [\"#00000000\", \"transparent\"]\na = [\"#102030\", \"auto_102030\"]\n\n" +
		"[[layers]]\nname = \"main\"\n\n[[frames]]\nduration_ms = 100\n\n" +
		"[[cels]]\nlayer = \"main\"\nframe = 0\norigin = [0, 0]\ngrid = '''\naaaa\naaaa\n'''\n"
	if got := string(read(t, path)); got != want {
		t.Fatalf("document:\n%s\nwant:\n%s", got, want)
	}
	if exists(pxofile.LogPath(path)) {
		t.Fatal("new wrote a history log")
	}

	// It exists now, also for a dry run.
	for _, dry := range []bool{false, true} {
		o := opts()
		o.DryRun = dry
		_, err := b.Create(path, newCall(t, path), o)
		if e, ok := errors.AsType[*errs.Error](err); !ok || e.Code != errs.FileExists || e.Arg != "file" {
			t.Fatalf("dry %t: %v, want file_exists", dry, err)
		}
	}

	// --force replaces the document and removes its history, which belongs
	// to the old one.
	if _, err := b.Apply(path, []Step{step(t, "", "paint #ffffff 0,0")}, opts()); err != nil {
		t.Fatal(err)
	}
	if !exists(pxofile.LogPath(path)) {
		t.Fatal("no log after a paint")
	}
	o := opts()
	o.Force = true
	if _, err := b.Create(path, newCall(t, path, "--size", "2x2"), o); err != nil {
		t.Fatal(err)
	}
	d, err := b.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.W != 2 || d.Rev != 0 || len(d.Cels) != 0 || exists(pxofile.LogPath(path)) {
		t.Fatalf("after --force: %dx%d rev %d, %d cels, log %t", d.W, d.H, d.Rev, len(d.Cels), exists(pxofile.LogPath(path)))
	}

	// A directory is never replaced, and op errors write nothing.
	o.Force = true
	sub := filepath.Join(dir, "sub.pxo")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create(sub, newCall(t, "x.pxo"), o); errs.CodeOf(err) != errs.FileExists || exists(LockPath(sub)) {
		t.Fatalf("directory: %v, lock file %t", err, exists(LockPath(sub)))
	}
	other := filepath.Join(dir, "other.pxo")
	if _, err := b.Create(other, newCall(t, other, "--bg", "nosuch"), opts()); errs.CodeOf(err) != errs.UnknownColor || exists(other) {
		t.Fatalf("bad --bg: %v, exists %t", err, exists(other))
	}
	// A missing directory fails the same way with and without --dry-run.
	missing := filepath.Join(dir, "missing", "a.pxo")
	for _, dry := range []bool{false, true} {
		o := opts()
		o.DryRun = dry
		if _, err := b.Create(missing, newCall(t, "a.pxo"), o); errs.CodeOf(err) != errs.IO {
			t.Fatalf("missing directory, dry %t: %v", dry, err)
		}
	}
	// Without --force, an existing document leaves no lock file either.
	plain := filepath.Join(t.TempDir(), "plain.pxo")
	if err := os.WriteFile(plain, read(t, path), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create(plain, newCall(t, plain), opts()); errs.CodeOf(err) != errs.FileExists || exists(LockPath(plain)) {
		t.Fatalf("existing: %v, lock file %t", err, exists(LockPath(plain)))
	}
}

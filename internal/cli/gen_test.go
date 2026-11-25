package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/history"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// markOp is a test-only op: `mark <at> <color> [--size n]` paints an n x n
// square whose top-left corner is at. It lives on a private registry and
// never reaches ops.Default.
type markOp struct{}

func (markOp) Spec() ops.Spec {
	return ops.Spec{
		Name: "mark", Group: "test", Summary: "Paint a square", Mutates: true,
		Args: []ops.ArgSpec{
			{Name: "at", Kind: ops.KindPoint, Required: true, Help: "top-left corner"},
			{Name: "color", Kind: ops.KindColor, Required: true},
			{Name: "size", Short: "s", Kind: ops.KindInt, Flag: true, Default: "1", Help: "side length"},
			{Name: "style", Kind: ops.KindEnum, Flag: true, Enum: []string{"solid", "hollow"}, Default: "solid"},
		},
		Examples: []string{"mark 1,1 ink", "mark 2,2 #ff0000 --size 3"},
	}
}

func (markOp) Apply(ctx *ops.Ctx, args ops.Args) error {
	at := args.Point("at")
	if !ctx.In(at.X, at.Y) {
		return errs.New(errs.OutOfBounds, "%d,%d is outside the canvas", at.X, at.Y).
			WithHint("canvas is %dx%d", ctx.Doc.W, ctx.Doc.H).WithArg("at")
	}
	r, err := color.Resolve(ctx.Doc, args.String("color"))
	if err != nil {
		return err
	}
	n := args.Int("size")
	for y := at.Y; y < at.Y+n; y++ {
		for x := at.X; x < at.X+n; x++ {
			edge := x == at.X || y == at.Y || x == at.X+n-1 || y == at.Y+n-1
			if args.String("style") == "solid" || edge {
				ctx.Set(x, y, r.Index)
			}
		}
	}
	return nil
}

type subOp struct{ markOp }

func (subOp) Spec() ops.Spec {
	s := markOp{}.Spec()
	s.Name, s.Examples = "group sub", nil
	return s
}

func testConfig(env map[string]string) config {
	r := ops.NewRegistry()
	r.Register(markOp{})
	r.Register(subOp{})
	return config{
		ops: r, backend: engine.Direct{},
		getenv: func(k string) string { return env[k] },
		now:    func() time.Time { return time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC) },
	}
}

// writeDoc writes an 8x8 document with an "ink" entry (key o) to name in
// the working directory.
func writeDoc(t *testing.T, name string) {
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
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// session runs commands in order and records a transcript: each command,
// its exit code, stdout and stderr.
type session struct {
	t   *testing.T
	env map[string]string
	ops *ops.Registry // the test registry when nil
	out strings.Builder
}

func (s *session) run(line string, args ...string) int {
	s.t.Helper()
	var stdout, stderr bytes.Buffer
	cfg := testConfig(s.env)
	if s.ops != nil {
		cfg.ops = s.ops
	}
	code := execute(BuildInfo{Version: "v0.0.0-test", Commit: "abc1234", Date: "2026-11-25T09:00:00Z"},
		cfg, args, &stdout, &stderr)
	fmt.Fprintf(&s.out, "$ pxo %s\nexit %d\n", line, code)
	if stdout.Len() > 0 {
		fmt.Fprintf(&s.out, "--- stdout\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		fmt.Fprintf(&s.out, "--- stderr\n%s", stderr.String())
	}
	s.out.WriteString("\n")
	return code
}

// sh runs a command line split on spaces; arguments with spaces go through
// run directly.
func (s *session) sh(line string) int {
	s.t.Helper()
	return s.run(line, strings.Fields(line)...)
}

// TestTranscript drives generated commands end to end and compares the
// output, exit codes, document and log with golden files.
func TestTranscript(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	writeDoc(t, "sprite.pxo")
	s := &session{t: t, env: map[string]string{}}

	// Drawing, with each common flag.
	s.sh("mark 1,1 ink")
	s.run("mark 2,2 o --size 3 --style hollow --json -m 'hollow box'", "mark", "2,2", "o", "--size", "3", "--style", "hollow", "--json", "-m", "hollow box")
	s.sh("mark sprite.pxo 0,0 ink --dry-run --json")
	s.sh("mark 0,0 ink --dry-run")
	s.sh("mark 1,1 ink --json")
	s.sh("mark 6,6 ink -s=2 --as bot --expect-rev 2 --json")
	s.sh("group sub 7,0 ink --json")
	s.env["PIXOPHER_JSON"] = "1"
	s.env["PIXOPHER_AUTHOR"] = "agent"
	s.sh("mark 0,7 ink")
	s.sh("mark 0,6 ink --json=false")
	delete(s.env, "PIXOPHER_JSON")
	delete(s.env, "PIXOPHER_AUTHOR")

	// Errors: usage (1), domain (2), conflict (3), no file (4).
	s.sh("mark -2,-2 ink --json")
	s.sh("mark 1;1 ink --json")
	s.sh("mark 1,1")
	s.sh("mark 1,1 ink extra")
	s.sh("mark 1,1 ink --style dotted --json")
	s.sh("mark 1,1 ink --bogus --json")
	s.sh("mark 1,1 ink --json=1 --bogus")
	s.sh("mark 1,1 nosuch --json")
	s.sh("mark 1,1 ink --expect-rev 1 --json")
	s.sh("mark -f other.pxo 1,1 ink --json")
	s.sh("mark -f sprite.pxo sprite.pxo 1,1 ink")
	s.sh("nope --json")
	s.sh("nope")
	s.sh("version --bogus --json")

	// File resolution: PIXOPHER_FILE, then `use`, then the only *.pxo.
	writeDoc(t, "other.pxo")
	s.sh("mark 1,1 ink --json")
	s.env["PIXOPHER_FILE"] = "other.pxo"
	s.sh("mark 3,3 ink --json")
	delete(s.env, "PIXOPHER_FILE")
	s.sh("use missing.pxo --json")
	s.sh("use other.pxo")
	s.sh("mark 4,4 ink --json")
	s.sh("use sprite.pxo --json")
	s.sh("--json -f other.pxo mark 5,5 ink")

	// Help is generated from the spec.
	s.sh("mark --help")
	s.sh("version --json")

	golden.Check(t, filepath.Join(testdata, "transcript.golden"), []byte(s.out.String()))
	for _, f := range []string{"sprite.pxo", "sprite.pxo.log", "other.pxo", "other.pxo.log"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		golden.Check(t, filepath.Join(testdata, "transcript."+f), data)
	}
}

// TestBuiltinTranscript drives the built-in drawing ops from new onward and
// compares the output, the document and its log with golden files.
func TestBuiltinTranscript(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	s := &session{t: t, env: map[string]string{}, ops: ops.Default}

	s.sh("new hero.pxo --size 16x16 --palette none --json")
	s.sh("new hero.pxo")
	s.sh("new hero --json")
	s.sh("new big.pxo --size 2000x10 --json")
	s.sh("new other.pxo --size 4x4 --bg #1a1c2c --dry-run --json")
	s.sh("rect 4,6 8,6 #ef7d57 --json")
	s.sh("rect 3,5,10,8 #1a1c2c --outline --json")
	s.sh("px 6,8 #1a1c2c 9,8 #1a1c2c 7,8 #f4f4f4 --json -m eyes")
	s.sh("line 0,0 15,3 #38b764 --width 2 --json")
	s.sh("line 0,15 15,13 #1a1c2c")
	s.sh("fill 8,10 #b13e53 --json")
	s.sh("fill 0,8 #29366f --8 --json")
	s.sh("erase 3,5 2,2 --json")
	s.sh("erase -5,-5")
	s.sh("replace #f4f4f4 #a7f070 --json")
	s.sh("clear --dry-run --json")

	// Observation.
	s.sh("info")
	s.sh("info --json")
	s.sh("info hero.pxo --json")
	s.sh("info -f hero.pxo")
	s.env["PIXOPHER_JSON"] = "1"
	s.sh("info")
	delete(s.env, "PIXOPHER_JSON")

	// Errors.
	s.sh("px 16,0 #000000 --json")
	s.sh("px 1,1 #000000 2,2 --json")
	s.sh("px 1,1 nosuch --json")
	s.sh("line 0,0 3,3 #000000 --width 0 --json")
	s.sh("fill 0,99 #000000")
	s.sh("new -f hero.pxo x.pxo --json")
	s.sh("info nosuch.pxo --json")
	s.sh("info extra --json")
	s.sh("info -m note --json")
	s.sh("info --dry-run")
	s.sh("info --expect-rev 3 --json")

	s.sh("px --help")
	s.sh("new --help")
	s.sh("info --help")

	// Palettes: new starts from the default palette unless told otherwise.
	s.sh("new pal.pxo --size 4x4 --palette nosuch --json")
	s.sh("new pal.pxo --size 4x4 --bg ink --json")
	s.sh("palette list pal.pxo")
	s.sh("use pal.pxo")
	s.sh("palette add k #ffcc99 skin --json")
	s.sh("palette add k #000000 other --json")
	s.sh("palette set skin #ffd0a0")
	s.sh("palette rename skin flesh --json")
	s.sh("ramp add flesh c k y --base k --json")
	s.sh("px 0,0 flesh 1,0 flesh-1 2,0 flesh+1")
	s.sh("palette rm flesh --json")
	s.sh("ramp rm flesh")
	s.sh("palette rm flesh --json")
	s.sh("palette rm flesh --replace coral --json")
	s.sh("undo --json")
	s.sh("redo --json")
	s.sh("palette list --json")
	s.sh("ramp rm nosuch --json")
	s.sh("palette --help")
	s.sh("palette rm --help")
	s.sh("new sweet.pxo --size 2x2 --palette sweetie16 --bg 0")
	s.sh("palette list sweet.pxo --json")

	golden.Check(t, filepath.Join(testdata, "builtin.golden"), []byte(s.out.String()))
	for _, f := range []string{"hero.pxo", "hero.pxo.log", "pal.pxo", "pal.pxo.log"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		golden.Check(t, filepath.Join(testdata, "builtin."+f), data)
	}
	if _, err := os.Stat("other.pxo"); err == nil {
		t.Fatal("new --dry-run wrote other.pxo")
	}
}

func TestGeneratorRejects(t *testing.T) {
	clash := func(a ops.ArgSpec) ops.Op {
		s := markOp{}.Spec()
		s.Args = append(s.Args, a)
		return specOp{s}
	}
	for name, op := range map[string]ops.Op{
		"name clash":  clash(ops.ArgSpec{Name: "message", Kind: ops.KindString, Flag: true}),
		"short clash": clash(ops.ArgSpec{Name: "fill", Short: "f", Kind: ops.KindBool, Flag: true}),
		"help clash":  clash(ops.ArgSpec{Name: "hollow", Short: "h", Kind: ops.KindBool, Flag: true}),
	} {
		t.Run(name, func(t *testing.T) {
			r := ops.NewRegistry()
			r.Register(op)
			defer func() {
				if recover() == nil {
					t.Fatal("newRoot did not panic")
				}
			}()
			cfg := testConfig(nil)
			cfg.ops = r
			newRoot(BuildInfo{}, &state{cfg: cfg})
		})
	}
}

type specOp struct{ spec ops.Spec }

func (o specOp) Spec() ops.Spec               { return o.spec }
func (specOp) Apply(*ops.Ctx, ops.Args) error { return nil }

func TestDefaultRegistryBuilds(*testing.T) {
	// Every built-in op must turn into a command without panicking.
	NewRoot(BuildInfo{})
}

// loadOnly is a backend that only loads documents; any other call fails
// the test.
type loadOnly struct {
	engine.Direct
	t *testing.T
}

func (b loadOnly) Apply(string, []engine.Step, engine.Options) (engine.Result, error) {
	b.t.Fatal("observation op called Apply")
	return engine.Result{}, nil
}

func (b loadOnly) Create(string, ops.Call, engine.Options) (engine.Result, error) {
	b.t.Fatal("observation op called Create")
	return engine.Result{}, nil
}

func (b loadOnly) Undo(string, history.Request, engine.Options) (engine.Result, error) {
	b.t.Fatal("observation op called Undo")
	return engine.Result{}, nil
}

// TestObserveOnlyReads: an observation op only loads its document (D38),
// so the document stays byte for byte the same and no lock file or log
// appears.
func TestObserveOnlyReads(t *testing.T) {
	t.Chdir(t.TempDir())
	writeDoc(t, "hero.pxo")
	before, err := os.ReadFile("hero.pxo")
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(nil)
	cfg.ops, cfg.backend = ops.Default, loadOnly{t: t}
	var stdout, stderr bytes.Buffer
	if code := execute(BuildInfo{}, cfg, []string{"info", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s%s", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile("hero.pxo")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("info changed the document")
	}
	for _, f := range []string{engine.LockPath("hero.pxo"), pxofile.LogPath("hero.pxo")} {
		if _, err := os.Stat(f); err == nil {
			t.Fatalf("info created %s", f)
		}
	}
}

// TestWriteReport: "ok": true goes before the report's own fields, and an
// empty report is just {"ok":true}.
func TestWriteReport(t *testing.T) {
	for _, tt := range []struct {
		rep  ops.Report
		want string
	}{
		{emptyReport{}, `{"ok":true}` + "\n"},
		{&tagReport{Tag: "<a&b>"}, `{"ok":true,"tag":"<a&b>"}` + "\n"},
	} {
		var b bytes.Buffer
		if err := writeReport(&b, tt.rep); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Fatalf("got %q, want %q", b.String(), tt.want)
		}
	}
	var b bytes.Buffer
	if err := writeReport(&b, listReport{}); errs.CodeOf(err) != errs.IO {
		t.Fatalf("non-object report: err = %v", err)
	}
}

type emptyReport struct{}

func (emptyReport) Text() string { return "" }

type tagReport struct {
	Tag string `json:"tag"`
}

func (*tagReport) Text() string { return "" }

type listReport []int

func (listReport) Text() string { return "" }

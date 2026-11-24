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
	out strings.Builder
}

func (s *session) run(line string, args ...string) int {
	s.t.Helper()
	var stdout, stderr bytes.Buffer
	code := execute(BuildInfo{Version: "v0.0.0-test", Commit: "abc1234", Date: "2026-11-25T09:00:00Z"},
		testConfig(s.env), args, &stdout, &stderr)
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

func TestGeneratorRejects(t *testing.T) {
	clash := func(a ops.ArgSpec) ops.Op {
		s := markOp{}.Spec()
		s.Args = append(s.Args, a)
		return specOp{s}
	}
	observe := markOp{}.Spec()
	observe.Mutates = false
	for name, op := range map[string]ops.Op{
		"name clash":  clash(ops.ArgSpec{Name: "message", Kind: ops.KindString, Flag: true}),
		"short clash": clash(ops.ArgSpec{Name: "fill", Short: "f", Kind: ops.KindBool, Flag: true}),
		"help clash":  clash(ops.ArgSpec{Name: "hollow", Short: "h", Kind: ops.KindBool, Flag: true}),
		"observation": specOp{observe},
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

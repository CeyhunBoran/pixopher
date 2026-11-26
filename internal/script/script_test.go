package script

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

func TestFields(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"rect 6,1 4,2 cork", []string{"rect", "6,1", "4,2", "cork"}},
		{"  px\t1,1  ink  ", []string{"px", "1,1", "ink"}},
		{"px 1,1 #ff8800", []string{"px", "1,1", "#ff8800"}},
		{"px 1,1 ink # the eye", []string{"px", "1,1", "ink"}},
		{"px 1,1 ink #", []string{"px", "1,1", "ink"}},
		{"px 1,1 a#b", []string{"px", "1,1", "a#b"}},
		{"x 'two words' \"say \\\"hi\\\"\"", []string{"x", "two words", `say "hi"`}},
		{`x 'it'\''s'`, []string{"x", "it's"}},
		{`x "a\nb" 'a\b'`, []string{"x", `a\nb`, `a\b`}},
		{`x '' ""`, []string{"x", "", ""}},
		{`x 'a # b' a\ b`, []string{"x", "a # b", "a b"}},
		{"", nil},
	}
	for _, tt := range tests {
		got, err := Fields(tt.in)
		if err != nil {
			t.Errorf("Fields(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Fields(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{`x 'open`, `x "open`, `x \`, `x "a\"`} {
		if _, err := Fields(in); errs.CodeOf(err) != errs.Usage {
			t.Errorf("Fields(%q): got %v, want a usage error", in, err)
		}
	}
}

// lines returns each step's message and canonical lines with their line
// numbers.
func lines(steps []Step) []string {
	var out []string
	for _, st := range steps {
		out = append(out, "--- "+st.Msg)
		for _, c := range st.Calls {
			out = append(out, strings.Join([]string{itoa(c.LineNo), c.Line}, ": "))
		}
	}
	return out
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestParseSteps(t *testing.T) {
	src := "# a potion\r\n" +
		"rect 6,1 4,2 #ff8800\r\n" +
		"\n" +
		"px 1,1 ink   # eye\n" +
		"--- the neck\n" +
		"  @set frame=0 layer=main\n" +
		"line 5,3 5,5 ink --width 1\n" +
		"---\n" +
		"---\tempty steps are dropped\n" +
		"--- \n" +
		"palette add k '#ffcc99' skin\n" +
		"---- not a step"
	steps, err := Parse(src, ops.Default, false)
	if err == nil {
		t.Fatalf("parsed ---- as a step: %q", lines(steps))
	}
	if e, _ := errors.AsType[*errs.Error](err); e == nil || e.Line != 12 || e.Cause != errs.Usage {
		t.Fatalf("got %#v, want a usage script_error on line 12", err)
	}
	steps, err = Parse(strings.TrimSuffix(src, "---- not a step"), ops.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--- ",
		"2: rect 6,1,4,2 #ff8800",
		"4: px 1,1 ink",
		"--- the neck",
		"6: @set layer=main frame=0",
		"7: line 5,3 5,5 ink",
		"--- ",
		"11: palette add k #ffcc99 skin",
	}
	if got := lines(steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		src, msg, hint string
		cause          errs.Code
	}{
		{"rect 1,1 2,2 ink\n\nrectt 1,1 2,2 ink", `line 3: usage: unknown op "rectt"`, `did you mean "rect"?`, errs.Usage},
		{"palette ad k #fff skin", `line 1: usage: unknown op "palette ad"`, `did you mean "palette add"?`, errs.Usage},
		{"undo", `line 1: usage: unknown op "undo"`, "", errs.Usage},
		{"show --grid", "line 1: usage: show doesn't change the document, so it can't run in a script", "run `pxo show` after the script", errs.Usage},
		{"new a.pxo", "line 1: usage: new makes a new document, so it can't run in a script", "run `pxo new` before the script", errs.Usage},
		{"line 1;1 2,2 ink", `line 1: usage: bad point "1;1" for from`, "", errs.Usage},
		{"px 1,1 ink -m hi", "line 1: usage: unknown flag -m", "", errs.Usage},
		{"px 1,1 'ink", "line 1: usage: unterminated ' quote", "close the quote on the same line", errs.Usage},
		{"@set", "line 1: usage: @set needs layer=<name|index> or frame=<index>", "", errs.Usage},
		{"@set layer=main layer=x", "line 1: usage: @set gives layer twice", "", errs.Usage},
		{"@set colour=red", `line 1: usage: @set takes layer=<name|index> and frame=<index>, not "colour=red"`, "", errs.Usage},
		{"@set layer=a-b", `line 1: usage: layer="a-b" is not a layer name or index`, "", errs.Usage},
		{"@set frame=-1", `line 1: usage: frame="-1" is not a frame index or tag:<name>`, "", errs.Usage},
		{"@sets layer=main", `line 1: usage: unknown directive "@sets"`, "the only directive is @set layer=<name|index> frame=<index>", errs.Usage},
	}
	for _, tt := range tests {
		_, err := Parse(tt.src, ops.Default, false)
		e, ok := errors.AsType[*errs.Error](err)
		if !ok || e.Code != errs.ScriptError || e.Cause != tt.cause || e.Message != tt.msg || tt.hint != "" && e.Hint != tt.hint {
			t.Errorf("Parse(%q): got %#v\nwant %s (hint %q)", tt.src, err, tt.msg, tt.hint)
		}
	}
}

func TestParseKeepGoing(t *testing.T) {
	// A line that can't be read becomes a call that fails with the plain
	// error when applied; the engine adds the line.
	steps, err := Parse("px 1,1 ink\nbogus 1\n---\nrect 1;1 2,2 ink", ops.Default, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--- ", "1: px 1,1 ink", "2: bogus 1", "--- ", "4: rect 1;1 2,2 ink"}
	if got := lines(steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	err = steps[0].Calls[1].Op.Apply(nil, nil)
	if e, _ := errors.AsType[*errs.Error](err); e == nil || e.Code != errs.Usage || e.Message != `unknown op "bogus"` {
		t.Fatalf("got %#v", err)
	}
}

// TestRoundTrip: every example of every op a script can call reads back
// from its canonical line to the same arguments and line.
func TestRoundTrip(t *testing.T) {
	n := 0
	for _, op := range ops.Default.All() {
		s := op.Spec()
		if !s.Mutates || s.Creates {
			continue
		}
		for _, ex := range s.Examples {
			first, err := Parse(ex, ops.Default, false)
			if err != nil {
				t.Errorf("%s: %v", ex, err)
				continue
			}
			c := first[0].Calls[0]
			again, err := Parse(c.Line, ops.Default, false)
			if err != nil {
				t.Errorf("%s: canonical %q: %v", ex, c.Line, err)
				continue
			}
			if d := again[0].Calls[0]; d.Line != c.Line || !reflect.DeepEqual(d.Args, c.Args) {
				t.Errorf("%s: %q read back as %q %v", ex, c.Line, d.Line, d.Args)
			}
			n++
		}
	}
	if n < 10 {
		t.Fatalf("only %d examples checked", n)
	}
}

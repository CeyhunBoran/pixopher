package ops

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// shapeSpec covers every value kind the parser supports.
var shapeSpec = Spec{
	Name: "shape", Group: "test", Summary: "test op", Mutates: true,
	Args: []ArgSpec{
		{Name: "at", Kind: KindPoint, Required: true},
		{Name: "box", Kind: KindRect, Required: true},
		{Name: "color", Kind: KindColor},
		{Name: "pad", Kind: KindInt, Default: "0"},
		{Name: "fill", Kind: KindBool, Flag: true},
		{Name: "smooth", Kind: KindBool, Flag: true, Default: "true"},
		{Name: "width", Short: "w", Kind: KindInt, Flag: true, Default: "1"},
		{Name: "dir", Kind: KindEnum, Flag: true, Enum: []string{"down", "right"}, Default: "down"},
		{Name: "size", Kind: KindSize, Flag: true},
		{Name: "clip", Kind: KindRect, Flag: true},
		{Name: "label", Kind: KindString, Flag: true},
	},
}

var repeatSpec = Spec{
	Name: "dots", Group: "test", Summary: "test op", Mutates: true,
	Args: []ArgSpec{
		{Name: "color", Kind: KindColor, Required: true},
		{Name: "at", Kind: KindPoint, Required: true, Repeat: true},
	},
}

func TestParse(t *testing.T) {
	tests := []struct {
		spec Spec
		in   string
		want Args
		line string
	}{
		{
			spec: shapeSpec,
			in:   "-2,-3 1,2 3x4",
			want: Args{"at": doc.Point{X: -2, Y: -3}, "box": doc.Rect{X: 1, Y: 2, W: 3, H: 4}, "pad": 0, "smooth": true, "width": 1, "dir": "down"},
			line: "shape -2,-3 1,2,3,4",
		},
		{
			spec: shapeSpec,
			in:   "--fill 0,0 -w 3 0,0,2,2 ink --smooth=false 2 --dir=right --size 8x8 --clip 1,1,2,2 --label it's",
			want: Args{
				"at": doc.Point{}, "box": doc.Rect{W: 2, H: 2}, "color": "ink", "pad": 2,
				"fill": true, "smooth": false, "width": 3, "dir": "right", "size": Size{W: 8, H: 8},
				"clip": doc.Rect{X: 1, Y: 1, W: 2, H: 2}, "label": "it's",
			},
			line: `shape 0,0 0,0,2,2 ink 2 --fill --smooth=false --width 3 --dir right --size 8,8 --clip 1,1,2,2 --label 'it'\''s'`,
		},
		{
			spec: shapeSpec,
			in:   "1,1 1,1,1,1 -- -x",
			want: Args{"at": doc.Point{X: 1, Y: 1}, "box": doc.Rect{X: 1, Y: 1, W: 1, H: 1}, "color": "-x", "pad": 0, "smooth": true, "width": 1, "dir": "down"},
			line: "shape 1,1 1,1,1,1 -x",
		},
		{
			spec: repeatSpec,
			in:   "ink 1,1 2,2 -3,4",
			want: Args{"color": "ink", "at": []doc.Point{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: -3, Y: 4}}},
			line: "dots ink 1,1 2,2 -3,4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.spec.Args, strings.Fields(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v\nwant %#v", got, tt.want)
			}
			if line := Format(tt.spec, got); line != tt.line {
				t.Fatalf("Format = %s, want %s", line, tt.line)
			}
		})
	}
}

// pairSpec has a two-member repeat group after a fixed positional (D40).
// A rect or area positional takes the next token when it is a size, so the
// tests start the group with points that are not sizes (a 0 or a negative).
var pairSpec = Spec{
	Name: "pairs", Group: "test", Summary: "test op", Mutates: true,
	Args: []ArgSpec{
		{Name: "box", Kind: KindArea, Required: true},
		{Name: "at", Kind: KindPoint, Required: true, Repeat: true},
		{Name: "color", Kind: KindColor, Required: true, Repeat: true},
		{Name: "clip", Kind: KindArea, Flag: true},
	},
}

func TestParseGroups(t *testing.T) {
	tests := []struct {
		in   string
		want Args
		line string
	}{
		{
			in:   "1,1 0,2 ink",
			want: Args{"box": doc.Rect{X: 1, Y: 1, W: 1, H: 1}, "at": []doc.Point{{X: 0, Y: 2}}, "color": []string{"ink"}},
			line: "pairs 1,1 0,2 ink",
		},
		{
			// x,y w,h takes the size; the group follows, interleaved.
			in: "0,0 3x2 1,1 ink -2,5 #ff8800 --clip 1,2,3,4",
			want: Args{
				"box": doc.Rect{W: 3, H: 2}, "at": []doc.Point{{X: 1, Y: 1}, {X: -2, Y: 5}},
				"color": []string{"ink", "#ff8800"}, "clip": doc.Rect{X: 1, Y: 2, W: 3, H: 4},
			},
			line: "pairs 0,0,3,2 1,1 ink -2,5 #ff8800 --clip 1,2,3,4",
		},
		{
			in:   "4,4,1,1 0,0 . --clip 7,7",
			want: Args{"box": doc.Rect{X: 4, Y: 4, W: 1, H: 1}, "at": []doc.Point{{}}, "color": []string{"."}, "clip": doc.Rect{X: 7, Y: 7, W: 1, H: 1}},
			line: "pairs 4,4 0,0 . --clip 7,7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(pairSpec.Args, strings.Fields(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v\nwant %#v", got, tt.want)
			}
			line := Format(pairSpec, got)
			if line != tt.line {
				t.Fatalf("Format = %s, want %s", line, tt.line)
			}
			back, err := Parse(pairSpec.Args, strings.Fields(line)[1:])
			if err != nil || !reflect.DeepEqual(back, got) {
				t.Fatalf("%s parsed back to %#v, %v", line, back, err)
			}
		})
	}
}

func TestParseGroupErrors(t *testing.T) {
	tests := []struct{ in, arg, msg string }{
		{"1,1", "at", "missing argument <at>"},
		{"1,1 0,2", "color", "missing argument <color> to complete the last group"},
		{"1,1 0,2 ink 3,3", "color", "missing argument <color> to complete the last group"},
		{"1,1 0,2 ink ink", "at", `bad point "ink" for at`},
		{"1,1 0,2 ink --clip 1,1 2,2", "color", "missing argument <color> to complete the last group"},
		{"1048577,0 0,2 ink", "box", `bad area "1048577,0" for box`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := Parse(pairSpec.Args, strings.Fields(tt.in))
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.Usage || e.Arg != tt.arg || !strings.HasPrefix(e.Message, tt.msg) {
				t.Fatalf("got %v (%#v), want usage %q on %q", err, e, tt.msg, tt.arg)
			}
		})
	}
}

func TestFormatRoundTrip(t *testing.T) {
	// Format's output, split into tokens the way a shell would, parses back
	// to the same Args.
	a := Args{
		"at": doc.Point{X: 3, Y: -1}, "box": doc.Rect{X: -1, Y: 0, W: 5, H: 6}, "color": "skin+1", "pad": 4,
		"fill": true, "smooth": false, "width": 2, "dir": "right", "size": Size{W: 1, H: 9},
		"clip": doc.Rect{W: 1, H: 1}, "label": "#ff8800",
	}
	line := Format(shapeSpec, a)
	got, err := Parse(shapeSpec.Args, strings.Fields(line)[1:])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, a) {
		t.Fatalf("%s parsed back to %#v", line, got)
	}
}

func TestFormatKeepsMiddleDefault(t *testing.T) {
	s := Spec{Name: "x", Group: "g", Summary: "s", Args: []ArgSpec{
		{Name: "a", Kind: KindInt, Default: "1"},
		{Name: "b", Kind: KindInt},
	}}
	if got := Format(s, Args{"a": 1, "b": 2}); got != "x 1 2" {
		t.Fatalf("got %q", got)
	}
	if got := Format(s, Args{"a": 1}); got != "x" {
		t.Fatalf("got %q", got)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"ink":       "ink",
		"#ff8800":   "#ff8800",
		"skin:-1":   "skin:-1",
		"":          "''",
		"#":         "'#'",
		"two words": "'two words'",
		"it's":      `'it'\''s'`,
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		in, arg, msg string
	}{
		{"", "at", "missing argument <at>"},
		{"1,1", "box", "missing argument <box>"},
		{"1;1 0,0,1,1", "at", `bad point "1;1" for at`},
		{"1,1 0,0,0,1", "box", `bad rect "0,0,0,1" for box`},
		{"1,1 0,0 0,1", "box", `bad rect "0,0 0,1" for box`},
		{"1,1 0,0", "box", `bad rect "0,0" for box`},
		{"1,1 0,0,1,1 ink x", "pad", `bad int "x" for pad`},
		{"1,1 0,0,1,1 ink 1 extra", "", `unexpected argument "extra"`},
		{"1,1 0,0,1,1 --dir up", "dir", `bad dir "up"`},
		{"1,1 0,0,1,1 --nope", "", "unknown flag --nope"},
		{"1,1 0,0,1,1 -q=1", "", "unknown flag -q"},
		{"1,1 0,0,1,1 --width", "width", "--width needs a int value"},
		{"1,1 0,0,1,1 -w 1 --width 2", "width", "--width is given twice"},
		{"1,1 0,0,1,1 --fill=maybe", "fill", `bad bool "maybe" for fill`},
		{"1,1 0,0,1,1 --size 0x4", "size", `bad size "0x4" for size`},
		{"1,1 0,0,1,1 --clip 0,0", "clip", `bad rect "0,0" for clip`},
		{"1,1 0,0,1,1 ''", "color", "empty color for color"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			toks := strings.Fields(tt.in)
			for i, s := range toks {
				if s == "''" {
					toks[i] = ""
				}
			}
			_, err := Parse(shapeSpec.Args, toks)
			e, ok := errors.AsType[*errs.Error](err)
			if !ok {
				t.Fatalf("err = %v, want *errs.Error", err)
			}
			if e.Code != errs.Usage || e.Arg != tt.arg || e.Message != tt.msg {
				t.Fatalf("got %s %q arg %q, want usage %q arg %q", e.Code, e.Message, e.Arg, tt.msg, tt.arg)
			}
		})
	}
}

func TestParseRepeatNeedsOne(t *testing.T) {
	if _, err := Parse(repeatSpec.Args, []string{"ink"}); errs.CodeOf(err) != errs.Usage {
		t.Fatalf("err = %v", err)
	}
}

func TestParseGridUnsupported(t *testing.T) {
	_, err := Parse([]ArgSpec{{Name: "g", Kind: KindGrid, Required: true}}, []string{"ab"})
	if errs.CodeOf(err) != errs.Usage {
		t.Fatalf("err = %v", err)
	}
}

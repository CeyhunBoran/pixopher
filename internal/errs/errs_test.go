package errs

import (
	"encoding/json"
	"fmt"
	"testing"

	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func TestExitCode(t *testing.T) {
	for c, want := range map[Code]int{
		Usage: 1, ParseError: 2, InvalidColor: 2, UnknownColor: 2, OutOfBounds: 2, TooLarge: 2,
		RevConflict: 3, Locked: 3, UndoConflict: 3, IO: 4, NoFile: 4, ServerUnavailable: 4,
	} {
		if got := ExitCode(c); got != want {
			t.Errorf("ExitCode(%s) = %d, want %d", c, got, want)
		}
	}
}

func TestCodeOf(t *testing.T) {
	e := New(InvalidColor, "bad %q", "x").WithHint("try %s", "y").WithArg("color")
	if e.Error() != `bad "x"` || e.Hint != "try y" || e.Arg != "color" {
		t.Fatalf("got %+v", e)
	}
	if CodeOf(fmt.Errorf("wrapped: %w", e)) != InvalidColor {
		t.Fatal("CodeOf does not unwrap")
	}
	if CodeOf(fmt.Errorf("plain")) != "" {
		t.Fatal("plain errors have no code")
	}
}

func TestJSON(t *testing.T) {
	full := New(OutOfBounds, "x=40 is outside the canvas").WithHint("canvas is 32x32; valid x is 0..31").WithArg("x")
	b, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"out_of_bounds","message":"x=40 is outside the canvas","hint":"canvas is 32x32; valid x is 0..31","arg":"x"}`
	if string(b) != want {
		t.Fatalf("got %s, want %s", b, want)
	}
	b, _ = json.Marshal(New(Locked, "busy"))
	if string(b) != `{"code":"locked","message":"busy"}` {
		t.Fatalf("empty hint and arg are not omitted: %s", b)
	}
}

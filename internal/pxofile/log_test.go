package pxofile

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

func sortDeltas(ds []doc.PixelDelta) {
	slices.SortFunc(ds, func(a, b doc.PixelDelta) int {
		switch {
		case a.Less(b):
			return -1
		case b.Less(a):
			return 1
		}
		return 0
	})
}

func TestPixelsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for n := range 50 {
		w, h := 1+rng.IntN(70), 1+rng.IntN(70)
		seen := map[[4]int]bool{}
		var ds []doc.PixelDelta
		for range rng.IntN(300) {
			p := doc.PixelDelta{
				Layer: rng.IntN(3), Frame: rng.IntN(3), X: rng.IntN(w), Y: rng.IntN(h),
				Before: doc.Index(rng.IntN(4)), After: doc.Index(4 + rng.IntN(300)),
			}
			k := [4]int{p.Layer, p.Frame, p.X, p.Y}
			if !seen[k] {
				seen[k] = true
				ds = append(ds, p)
			}
		}
		sortDeltas(ds)
		got, err := DecodePixels(EncodePixels(ds, w))
		if err != nil {
			t.Fatalf("case %d: %v", n, err)
		}
		if !reflect.DeepEqual(got, ds) {
			t.Fatalf("case %d: round trip differs", n)
		}
	}
}

// TestPixelsRuns: a filled area packs into runs, so a full 64x64 fill stays
// small, and the bytes are fixed (they are part of the log format).
func TestPixelsRuns(t *testing.T) {
	var ds []doc.PixelDelta
	for y := range 64 {
		for x := range 64 {
			ds = append(ds, doc.PixelDelta{X: x, Y: y, Before: 0, After: 1})
		}
	}
	got := EncodePixels(ds, 64)
	if got != "AQAAQAEAgCAAAQ==" {
		t.Fatalf("encoded fill = %q", got)
	}
	if EncodePixels(nil, 64) != "" {
		t.Fatal("no deltas should encode to an empty string")
	}
	if d, err := DecodePixels(""); d != nil || err != nil {
		t.Fatal("empty string should decode to nothing")
	}
}

func TestDecodePixelsErrors(t *testing.T) {
	for _, s := range []string{
		"!!!",                  // not base64
		"Ag==",                 // unknown version
		"AQA=",                 // truncated group
		"AQAAAAE=",             // zero width
		"AQAAQAEAAA==",         // zero-length run
		"AQAAQAEA/////w8BAAE=", // run beyond the maximum canvas
	} {
		if _, err := DecodePixels(s); errs.CodeOf(err) != errs.ParseError {
			t.Errorf("DecodePixels(%q) = %v, want parse_error", s, err)
		}
	}
}

func TestMetaRoundTrip(t *testing.T) {
	before := fullDoc(t, 1)
	after := before.Clone()
	after.Palette.Entries = append(after.Palette.Entries, doc.Entry{Key: "z", Name: "auto_123456", Color: doc.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}})
	m := &doc.MetaDelta{Before: before.Header(), After: after.Header()}
	snaps, err := EncodeMeta(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snaps[0].Before, "[[cels]]") || !strings.Contains(snaps[0].Before, "rev = 0") {
		t.Fatal("snapshot should hold the header only, at rev 0")
	}
	got, err := DecodeMeta(snaps)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]*doc.Doc{{got.Before, m.Before}, {got.After, m.After}} {
		if a, b := mustSerialize(t, pair[0]), mustSerialize(t, pair[1]); string(a) != string(b) {
			t.Fatalf("meta round trip differs:\n%s\n---\n%s", a, b)
		}
	}
	if s, _ := EncodeMeta(nil); s == nil || len(s) != 0 {
		t.Fatal("nil meta should encode to an empty, non-nil list")
	}
	if m, err := DecodeMeta(nil); m != nil || err != nil {
		t.Fatal("no snapshots should decode to nil")
	}
	if _, err := DecodeMeta([]MetaSnap{{}, {}}); errs.CodeOf(err) != errs.ParseError {
		t.Fatal("two snapshots should be a parse_error")
	}
}

func testEntries() []Entry {
	return []Entry{
		{Rev: 1, Tx: "t_0001", TS: "2026-11-24T10:00:00Z", Author: "cli", Msg: "<b> & co", Ops: []string{"px 1,1 o"}, BBox: []int{1, 1, 1, 1},
			Undo: Undo{Pixels: EncodePixels([]doc.PixelDelta{{X: 1, Y: 1, After: 1}}, 4)}},
		{Rev: 2, Tx: "t_0001", TS: "2026-11-24T10:00:00Z", Author: "cli", Ops: []string{"palette add x #fff"}},
	}
}

func TestLogAppendRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.pxo.log")
	if es, err := ReadLog(path); es != nil || err != nil {
		t.Fatalf("missing log: %v %v", es, err)
	}
	want := testEntries()
	if err := AppendLog(path, want[:1]); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(path, want[1:]); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	wantText := `{"rev":1,"tx":"t_0001","ts":"2026-11-24T10:00:00Z","author":"cli","msg":"<b> & co","ops":["px 1,1 o"],"bbox":[1,1,1,1],"undo":{"pixels":"AQAABAEFAQAB","meta":[]}}
{"rev":2,"tx":"t_0001","ts":"2026-11-24T10:00:00Z","author":"cli","ops":["palette add x #fff"],"undo":{"pixels":"","meta":[]}}
`
	if string(data) != wantText {
		t.Fatalf("log text:\n%s\nwant:\n%s", data, wantText)
	}
	got, err := ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	want[1].Undo.Meta = []MetaSnap{}
	want[0].Undo.Meta = []MetaSnap{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadLog = %+v\nwant %+v", got, want)
	}
}

// TestLogTornTail: a partial line from an interrupted append is ignored by
// ReadLog and cut off by the next AppendLog. A complete last line without a
// newline is kept.
func TestLogTornTail(t *testing.T) {
	dir := t.TempDir()
	es := testEntries()
	first, _ := MarshalEntries(es[:1])

	torn := filepath.Join(dir, "torn.pxo.log")
	if err := os.WriteFile(torn, append(slices.Clone(first), `{"rev":2,"tx":"t_`...), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLog(torn); err != nil || len(got) != 1 {
		t.Fatalf("ReadLog on torn log = %d entries, %v", len(got), err)
	}
	if err := AppendLog(torn, es[1:]); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLog(torn); err != nil || len(got) != 2 || got[1].Rev != 2 {
		t.Fatalf("after repair: %d entries, %v", len(got), err)
	}

	noNL := filepath.Join(dir, "nonl.pxo.log")
	if err := os.WriteFile(noNL, first[:len(first)-1], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(noNL, es[1:]); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLog(noNL); err != nil || len(got) != 2 {
		t.Fatalf("complete last line was dropped: %d entries, %v", len(got), err)
	}

	bad := filepath.Join(dir, "bad.pxo.log")
	if err := os.WriteFile(bad, append([]byte("not json\n"), first...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLog(bad); errs.CodeOf(err) != errs.ParseError || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("bad middle line: %v", err)
	}
}

// FuzzDecodePixels: arbitrary input never panics, and decoded positions
// stay within the decoder's limits. Checking deltas against a real document
// is the caller's job.
func FuzzDecodePixels(f *testing.F) {
	f.Add("AQAAQAEAgCAAAQ==")
	f.Add("AQAABAEFAQAB")
	f.Fuzz(func(t *testing.T, s string) {
		ds, err := DecodePixels(s)
		if err != nil {
			return
		}
		for _, p := range ds {
			if p.X < 0 || p.Y < 0 || p.X >= doc.MaxSize || p.Y >= doc.MaxSize*doc.MaxSize {
				t.Fatalf("delta out of range: %+v", p)
			}
		}
	})
}

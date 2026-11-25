package history

import (
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// pal builds a palette from "key:name" items; the color is derived from the
// name so that renaming and recoloring both show up as a different entry.
func pal(items string) []doc.Entry {
	out := []doc.Entry{{Key: ".", Name: doc.TransparentName}}
	for it := range strings.FieldsSeq(items) {
		k, n, _ := strings.Cut(it, ":")
		out = append(out, doc.Entry{Key: k, Name: n, Color: doc.RGBA{R: n[0], A: 255}})
	}
	return out
}

func keyList(es []doc.Entry) string {
	var ks []string
	for _, e := range es {
		ks = append(ks, e.Key+":"+e.Name)
	}
	return strings.Join(ks[1:], " ")
}

// TestMergePalette: a step's palette change (from want to to) applied to a
// palette that changed since, key by key.
func TestMergePalette(t *testing.T) {
	for _, tc := range []struct {
		name, want, to, cur, out string
		touched                  []string
	}{
		{"undo an add", "o:ink a:red", "o:ink", "o:ink a:red b:green", "o:ink b:green", []string{"a"}},
		{"redo an add", "o:ink", "o:ink a:red", "o:ink b:green", "o:ink a:red b:green", []string{"a"}},
		{"undo a rm", "o:ink b:blue", "o:ink a:red b:blue", "o:ink b:blue z:zed", "o:ink a:red b:blue z:zed", []string{"a"}},
		{"undo a rm, its neighbor gone", "o:ink b:blue", "o:ink a:red b:blue", "b:blue", "a:red b:blue", []string{"a"}},
		{"undo a recolor", "o:ink a:ruby", "o:ink a:red", "o:ink a:ruby b:green", "o:ink a:red b:green", []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			touched, reordered := touchedKeys(pal(tc.want), pal(tc.to))
			if reordered || !reflect.DeepEqual(touched, tc.touched) {
				t.Fatalf("touched %v reordered %v, want %v", touched, reordered, tc.touched)
			}
			if got := keyList(mergePalette(pal(tc.cur), pal(tc.to), touched)); got != tc.out {
				t.Fatalf("merged %q, want %q", got, tc.out)
			}
		})
	}
	if _, reordered := touchedKeys(pal("a:x b:y"), pal("b:y a:x")); !reordered {
		t.Fatal("swapped entries should count as reordered")
	}
}

// Package golden compares test output with golden files. Run
// `go test ./... -update` to rewrite them. Every package with tests imports
// it, so the -update flag exists in every test binary.
package golden

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// Update is set by the -update test flag.
var Update = flag.Bool("update", false, "rewrite golden files")

// Check compares got with the golden file at path, or rewrites the file when
// -update is set.
func Check(t testing.TB, path string, got []byte) {
	t.Helper()
	if *Update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (run go test -update to accept)\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

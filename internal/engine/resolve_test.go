package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

func touch(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Dir(n), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(n, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// wantCode fails unless err has code, and returns its message.
func wantCode(t *testing.T, err error, code errs.Code) string {
	t.Helper()
	e, ok := errors.AsType[*errs.Error](err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
	return e.Message
}

func TestResolveFileOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	touch(t, "a.pxo", "b.pxo", "sub/c.pxo")
	if _, err := Use("sub/c.pxo"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ flag, env, want string }{
		{"x.pxo", "y.pxo", "x.pxo"},
		{"", "y.pxo", "y.pxo"},
		{"", "", filepath.FromSlash("sub/c.pxo")},
	} {
		got, err := ResolveFile(tt.flag, tt.env)
		if err != nil || got != tt.want {
			t.Fatalf("ResolveFile(%q, %q) = %q, %v; want %q", tt.flag, tt.env, got, err, tt.want)
		}
	}
}

func TestResolveFileOnlyOne(t *testing.T) {
	t.Chdir(t.TempDir())
	// Directories, other extensions, a bare ".pxo" and an upper-case
	// extension don't count.
	touch(t, "notes.txt", "sprite.pxo", ".pxo", "OLD.PXO", "dir.pxo/x")
	got, err := ResolveFile("", "")
	if err != nil || got != "sprite.pxo" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolveFileNone(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := ResolveFile("", "")
	if msg := wantCode(t, err, errs.NoFile); msg != "no document given and no .pxo file in the working directory" {
		t.Fatalf("message %q", msg)
	}
}

func TestResolveFileSeveral(t *testing.T) {
	t.Chdir(t.TempDir())
	touch(t, "b.pxo", "a.pxo", "c.pxo")
	_, err := ResolveFile("", "")
	if msg := wantCode(t, err, errs.NoFile); msg != "no document given and 3 .pxo files in the working directory: a.pxo, b.pxo, c.pxo" {
		t.Fatalf("message %q", msg)
	}
}

func TestResolveFileStaleCurrent(t *testing.T) {
	t.Chdir(t.TempDir())
	touch(t, "a.pxo", "gone.pxo")
	if _, err := Use("gone.pxo"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("gone.pxo"); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveFile("", "")
	wantCode(t, err, errs.NoFile)
}

func TestResolveFileEmptyCurrent(t *testing.T) {
	t.Chdir(t.TempDir())
	touch(t, "a.pxo", CurrentFile)
	got, err := ResolveFile("", "")
	if err != nil || got != "a.pxo" {
		t.Fatalf("an empty current file should be ignored: %q, %v", got, err)
	}
}

func TestUse(t *testing.T) {
	t.Chdir(t.TempDir())
	touch(t, "art/hero.pxo")
	got, err := Use(filepath.FromSlash("./art/../art/hero.pxo"))
	if err != nil || got != "art/hero.pxo" {
		t.Fatalf("Use = %q, %v", got, err)
	}
	data, err := os.ReadFile(CurrentFile)
	if err != nil || string(data) != "art/hero.pxo\n" {
		t.Fatalf("current file %q, %v", data, err)
	}
	_, err = Use("missing.pxo")
	wantCode(t, err, errs.NoFile)
	_, err = Use("art")
	wantCode(t, err, errs.NoFile)
}

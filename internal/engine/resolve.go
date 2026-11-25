package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// CurrentFile is where `use` stores the current document, relative to the
// working directory.
const CurrentFile = ".pixopher/current"

// Ext is the document file extension. It is matched case-sensitively.
const Ext = ops.Ext

// ResolveFile picks the target document (docs/cli.md#file-resolution-done):
// flag (-f or a leading .pxo positional), then env (PIXOPHER_FILE), then
// the file stored by Use in the working directory, then the only *.pxo file
// in the working directory. It fails with no_file listing the candidates.
// It doesn't check that a flag or env path exists; loading it does.
func ResolveFile(flag, env string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env != "" {
		return env, nil
	}
	if p, err := readCurrent(); err != nil || p != "" {
		return p, err
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		return "", ioErr(err)
	}
	var found []string
	for _, e := range entries { // sorted by name
		if !e.IsDir() && strings.HasSuffix(e.Name(), Ext) && len(e.Name()) > len(Ext) {
			found = append(found, e.Name())
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", errs.New(errs.NoFile, "no document given and no %s file in the working directory", Ext).
			WithHint("pass -f <file>, or create one with `pxo new sprite.pxo --size 32x32`")
	}
	return "", errs.New(errs.NoFile, "no document given and %d %s files in the working directory: %s",
		len(found), Ext, strings.Join(found, ", ")).
		WithHint("pass -f <file>, or pick one for this directory with `pxo use <file>`")
}

// readCurrent returns the path stored by Use, or "" when there is none.
func readCurrent() (string, error) {
	data, err := os.ReadFile(filepath.FromSlash(CurrentFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", ioErr(err)
	}
	p := strings.TrimSpace(string(data))
	if p == "" {
		return "", nil
	}
	p = filepath.FromSlash(p)
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		return "", errs.New(errs.NoFile, "%s (set by `pxo use`) does not exist", p).
			WithHint("run `pxo use <file>` again, or create it with `pxo new %s --size 32x32`", p)
	}
	return p, nil
}

// Use makes path the current document for the working directory by storing
// it in CurrentFile. The path is stored cleaned, with forward slashes; a
// relative path stays relative to the working directory. Use returns the
// stored path. The document must exist.
func Use(path string) (string, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", noFile(path)
	}
	if err != nil {
		return "", ioErr(err)
	}
	if fi.IsDir() {
		return "", errs.New(errs.NoFile, "%s is a directory, not a document", path).WithArg("file")
	}
	stored := filepath.ToSlash(filepath.Clean(path))
	cur := filepath.FromSlash(CurrentFile)
	if err := os.MkdirAll(filepath.Dir(cur), 0o755); err != nil {
		return "", ioErr(err)
	}
	if err := writeAtomic(cur, []byte(stored+"\n")); err != nil {
		return "", err
	}
	return stored, nil
}

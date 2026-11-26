package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// writeAtomic replaces path with data: it writes a temp file in the same
// directory, syncs it and renames it over path. Readers see either the old
// or the new file, never a partial one. The rename is atomic on POSIX and
// best-effort on Windows. An existing file keeps its permissions; a new one
// gets 0644.
func writeAtomic(path string, data []byte) (err error) {
	mode := fs.FileMode(0o644)
	if st, statErr := os.Stat(path); statErr == nil {
		mode = st.Mode().Perm()
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return writeErr(path, statErr)
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return writeErr(path, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return writeErr(path, err)
	}
	// os.CreateTemp makes the file 0600; restore the document's mode.
	if err = f.Chmod(mode); err != nil {
		return writeErr(path, err)
	}
	if err = syncFile(f); err != nil {
		return writeErr(path, err)
	}
	if err = f.Close(); err != nil {
		return writeErr(path, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return writeErr(path, err)
	}
	return nil
}

// writeErr is an io error for a failed write of path. It names path rather
// than the temp file, whose random name would make the message differ on
// every run, and states common causes the same way on every OS.
func writeErr(path string, err error) error {
	cause := err.Error()
	if errors.Is(err, fs.ErrPermission) {
		cause = "permission denied"
	} else if errors.Is(err, fs.ErrNotExist) {
		cause = "its directory does not exist"
	} else if pe, ok := errors.AsType[*fs.PathError](err); ok {
		cause = pe.Err.Error()
	} else if le, ok := errors.AsType[*os.LinkError](err); ok {
		cause = le.Err.Error()
	}
	return errs.New(errs.IO, "can't write %s: %s", path, cause)
}

// WriteFile replaces path with data atomically, like the document writes:
// readers never see a partial file. It is used for exports.
func WriteFile(path string, data []byte) error { return writeAtomic(path, data) }

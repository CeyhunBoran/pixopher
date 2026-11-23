package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
		return ioErr(statErr)
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return ioErr(err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return ioErr(err)
	}
	// os.CreateTemp makes the file 0600; restore the document's mode.
	if err = f.Chmod(mode); err != nil {
		return ioErr(err)
	}
	if err = syncFile(f); err != nil {
		return ioErr(err)
	}
	if err = f.Close(); err != nil {
		return ioErr(err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return ioErr(err)
	}
	return nil
}

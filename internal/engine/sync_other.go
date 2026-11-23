//go:build !darwin

package engine

import "os"

// syncFile flushes f to stable storage.
func syncFile(f *os.File) error { return f.Sync() }

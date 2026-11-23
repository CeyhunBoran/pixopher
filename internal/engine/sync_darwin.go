package engine

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncFile flushes f with a plain fsync. os.File.Sync uses F_FULLFSYNC on
// macOS, which can take longer than the whole 15 ms command budget.
func syncFile(f *os.File) error { return unix.Fsync(int(f.Fd())) }

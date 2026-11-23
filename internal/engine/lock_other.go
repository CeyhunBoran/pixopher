//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package engine

import "os"

// tryLock always succeeds on platforms without advisory locks (such as
// wasm). Writers are then not serialized; --expect-rev still detects
// conflicting writes.
func tryLock(*os.File) (bool, error) { return true, nil }

func unlock(*os.File) {}

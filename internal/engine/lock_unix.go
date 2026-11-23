//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package engine

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive flock without blocking.
func tryLock(f *os.File) (bool, error) {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}

func unlock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }

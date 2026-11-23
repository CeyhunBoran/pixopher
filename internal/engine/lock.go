package engine

import (
	"os"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// LockPath returns the lock file path of the document at path. The lock file
// is never deleted: removing it while another process waits on it would let
// two writers hold "the" lock at once.
func LockPath(path string) string { return path + ".lock" }

// lockPoll is how often a busy lock is retried.
const lockPoll = 5 * time.Millisecond

// fileLock is an exclusive advisory lock on a lock file.
type fileLock struct{ f *os.File }

// acquire takes the lock at path, retrying until timeout. A lock held by
// another writer for longer fails with `locked`.
func acquire(path string, timeout time.Duration) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, ioErr(err)
	}
	deadline := time.Now().Add(timeout)
	for {
		ok, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, ioErr(err)
		}
		if ok {
			return &fileLock{f}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, errs.New(errs.Locked, "%s is locked by another writer", path).
				WithHint("waited %s; retry, or check for a stuck pixopher process", timeout)
		}
		time.Sleep(lockPoll)
	}
}

// release drops the lock. Closing the file releases it on every platform.
func (l *fileLock) release() error {
	unlock(l.f)
	return l.f.Close()
}

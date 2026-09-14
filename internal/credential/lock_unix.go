//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package credential

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an advisory exclusive lock on file without waiting, and
// reports false when another process holds it.
func tryLock(file *os.File) (bool, error) {
	// A descriptor is small and non-negative.
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) // #nosec G115
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN) // #nosec G115
}

//go:build aix || solaris

package credential

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive record lock on the whole of file without
// waiting, and reports false when another process holds it. AIX, Solaris and
// illumos lack a usable flock; a record lock excludes other processes the same way,
// and since this process takes the lock only once at a time, its being shared
// within a process does not matter.
func tryLock(file *os.File) (bool, error) {
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart}
	err := unix.FcntlFlock(file.Fd(), unix.F_SETLK, &lock)
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EACCES) {
		return false, nil
	}
	return err == nil, err
}

func unlock(file *os.File) error {
	lock := unix.Flock_t{Type: unix.F_UNLCK, Whence: io.SeekStart}
	return unix.FcntlFlock(file.Fd(), unix.F_SETLK, &lock)
}

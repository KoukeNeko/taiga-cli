//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows)

package credential

import "os"

// tryLock always succeeds on the platforms left here (plan9, js, wasip1 and
// z/OS), none of which this CLI is built or supported for. Refusing the lock
// there would fail every refresh outright, which is worse than refreshes that
// are merely uncoordinated.
func tryLock(*os.File) (bool, error) { return true, nil }

func unlock(*os.File) error { return nil }

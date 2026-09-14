//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package credential

import "os"

// tryLock always succeeds where there is no file locking to use, which leaves
// concurrent refreshes uncoordinated there rather than failing every one.
func tryLock(*os.File) (bool, error) { return true, nil }

func unlock(*os.File) error { return nil }

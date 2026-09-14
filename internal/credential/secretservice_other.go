//go:build !((dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd)

package credential

// secretServiceMissing is false where go-keyring does not use the Secret
// Service: macOS and Windows always have their keyring, and a platform with
// none at all is recognised by go-keyring's own error.
func secretServiceMissing() bool { return false }

// headless is false where the keyring's prompt is not tied to a Linux
// desktop session.
func headless() bool { return false }

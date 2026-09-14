//go:build (dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd

package credential

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	dbus "github.com/godbus/dbus/v5"
)

// These build constraints are go-keyring's own for its Secret Service
// provider, so the question is asked exactly where that provider is in use.

const secretServiceName = "org.freedesktop.secrets"

// secretServiceMissing reports whether nothing provides the Secret Service:
// no session bus at all, or a bus on which no program owns the name and none
// can be started for it. The error go-keyring returns says as much only in
// words that differ by cause, so the bus is asked directly.
//
// Only proof of absence counts. A bus that is known but cannot be reached, as
// with a stale address or one inherited through sudo from another user, is a
// broken keyring rather than a missing one, and a keyring that exists but is
// broken must not quietly become a file.
func secretServiceMissing() bool {
	if !sessionBusKnown() {
		return true
	}
	// The shared connection would run dbus-launch when no bus is found, and
	// asking whether a bus exists should not start one.
	conn, err := dbus.SessionBusPrivateNoAutoStartup()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Auth(nil); err != nil {
		return false
	}
	if err := conn.Hello(); err != nil {
		return false
	}
	bus := conn.BusObject()
	var owned bool
	if err := bus.Call("org.freedesktop.DBus.NameHasOwner", 0, secretServiceName).Store(&owned); err != nil || owned {
		return false
	}
	var activatable []string
	if err := bus.Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&activatable); err != nil {
		return false
	}
	return !slices.Contains(activatable, secretServiceName)
}

// headless reports whether this session has no desktop. A Secret Service that
// needs unlocking shows its prompt on one, and without one the prompt fails at
// once, leaving an error that says nothing of why.
func headless() bool {
	return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// sessionBusKnown reports whether there is a session bus address to use,
// looking where godbus looks before it falls back to launching a bus. A bus
// that the failed keyring call launched itself counts too: godbus exports the
// launched bus's address to DBUS_SESSION_BUS_ADDRESS, so the probe asks that
// same bus, and a keyring activated there is found rather than taken for
// absent.
func sessionBusKnown() bool {
	if address := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); address != "" && address != "autolaunch:" {
		return true
	}
	runtimeDirectory := fmt.Sprintf("/run/user/%d", os.Getuid())
	for _, name := range []string{"bus", "dbus-session"} {
		if _, err := os.Stat(filepath.Join(runtimeDirectory, name)); err == nil {
			return true
		}
	}
	return false
}

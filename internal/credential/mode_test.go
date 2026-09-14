package credential

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMode(t *testing.T) {
	for input, want := range map[string]Mode{"": ModeAuto, "auto": ModeAuto, " File ": ModeFile, "KEYRING": ModeKeyring, "none": ModeNone} {
		if got, err := ParseMode(input); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v, want %q", input, got, err, want)
		}
	}
	_, err := ParseMode("vault")
	if err == nil || !strings.Contains(err.Error(), "auto, keyring, file, none") {
		t.Fatalf("ParseMode(vault) = %v, want the valid modes listed", err)
	}
}

func TestFileModeKeepsCredentialsOnlyInTheFile(t *testing.T) {
	directory := t.TempDir()
	store := NewStore(ModeFile, directory)
	want := Tokens{AuthToken: "auth", RefreshToken: "refresh"}
	path := filepath.Join(directory, FileName)
	if location, err := store.Set("account", want); err != nil || location.File != path {
		t.Fatalf("Set() = %#v, %v, want file %q", location, err, path)
	}
	got, location, err := store.Get("account")
	if err != nil || got != want || location.File != path {
		t.Fatalf("Get() = %#v, %#v, %v", got, location, err)
	}
	if err := store.Delete("account"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get("account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete = %v, want ErrNotFound", err)
	}
}

func TestNoneModeKeepsNothing(t *testing.T) {
	directory := t.TempDir()
	store := NewStore(ModeNone, directory)
	if _, err := store.Set("account", Tokens{AuthToken: "auth"}); !errors.Is(err, ErrStorageDisabled) {
		t.Fatalf("Set() = %v, want ErrStorageDisabled", err)
	}
	if _, _, err := store.Get("account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() = %v, want ErrNotFound", err)
	}
	if err := store.Delete("account"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 0 {
		t.Fatalf("none mode wrote %v", entries)
	}
}

// Keyring mode is for a person who would rather fail than have a token
// written to disk, so even a machine with no keyring at all gets an error.
func TestKeyringModeNeverFallsBackToTheFile(t *testing.T) {
	directory := t.TempDir()
	missing := errors.New("The name org.freedesktop.secrets was not provided by any .service files")
	store := &KeyringStore{
		backend:              &fakeBackend{values: map[string]string{}, getErr: missing, setErr: missing},
		secretServiceMissing: func() bool { return true },
		headless:             func() bool { return true },
	}
	_, err := store.Set("account", Tokens{AuthToken: "auth"})
	var keyringErr *KeyringError
	if !errors.As(err, &keyringErr) || !keyringErr.Headless || !errors.Is(err, missing) {
		t.Fatalf("Set() = %#v, want a headless KeyringError wrapping the keyring's error", err)
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 0 {
		t.Fatalf("keyring mode wrote %v", entries)
	}
}

func TestGetSaysTheCredentialCameFromTheFile(t *testing.T) {
	store, path := noKeyringStore(t)
	if _, err := store.Set("account", Tokens{AuthToken: "auth"}); err != nil {
		t.Fatal(err)
	}
	if _, location, err := store.Get("account"); err != nil || location.File != path {
		t.Fatalf("Get() location = %#v, %v, want %q", location, err, path)
	}
	backend := &fakeBackend{values: map[string]string{serviceName + "|keyring": `{"auth_token":"auth"}`}}
	if _, location, err := newKeyringStore(backend).Get("keyring"); err != nil || location.File != "" {
		t.Fatalf("Get() from the keyring = %#v, %v, want no file", location, err)
	}
}

func TestLockExcludesAnotherHolderUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockFileName)
	unlock, err := lockFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	waiting, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := lockFile(waiting, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock while held = %v, want it to wait until the deadline", err)
	}
	unlock()
	again, err := lockFile(context.Background(), path)
	if err != nil {
		t.Fatalf("lock after release = %v", err)
	}
	again()
}

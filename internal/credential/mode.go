package credential

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Mode is where credentials may be kept.
type Mode string

const (
	// ModeAuto uses the OS keyring, and the credentials file only where
	// there is provably no keyring.
	ModeAuto Mode = "auto"
	// ModeKeyring uses only the OS keyring, and fails when it cannot.
	ModeKeyring Mode = "keyring"
	// ModeFile uses only the credentials file and never asks for a keyring.
	ModeFile Mode = "file"
	// ModeNone keeps nothing, for callers that pass TAIGA_TOKEN.
	ModeNone Mode = "none"
)

// Modes lists every mode, in the order help text shows them.
var Modes = []Mode{ModeAuto, ModeKeyring, ModeFile, ModeNone}

// ErrStorageDisabled is what saving a credential returns in ModeNone.
var ErrStorageDisabled = errors.New("credential storage is disabled")

// ParseMode reads a mode as given on the command line or in the environment.
// An empty value is ModeAuto.
func ParseMode(value string) (Mode, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ModeAuto, nil
	}
	for _, mode := range Modes {
		if Mode(value) == mode {
			return mode, nil
		}
	}
	names := make([]string, len(Modes))
	for i, mode := range Modes {
		names[i] = string(mode)
	}
	return "", fmt.Errorf("unknown credential store %q; use one of %s", value, strings.Join(names, ", "))
}

// NewStore returns the store for mode, keeping its files in directory.
func NewStore(mode Mode, directory string) Store {
	file := &fileStore{path: filepath.Join(directory, FileName)}
	lockPath := filepath.Join(directory, lockFileName)
	switch mode {
	case ModeFile:
		return &FileStore{file: file, lockPath: lockPath}
	case ModeNone:
		return noStore{}
	case ModeKeyring:
		return &KeyringStore{backend: osBackend{}, lockPath: lockPath, headless: headless}
	default:
		return &KeyringStore{backend: osBackend{}, file: file, lockPath: lockPath, secretServiceMissing: secretServiceMissing, headless: headless}
	}
}

// noStore keeps nothing. Nothing is found in it, removing from it succeeds,
// and there is nothing to lock, since without a stored refresh token no
// refresh happens.
type noStore struct{}

func (noStore) Get(string) (Tokens, Location, error) { return Tokens{}, Location{}, ErrNotFound }

func (noStore) Set(string, Tokens) (Location, error) { return Location{}, ErrStorageDisabled }

func (noStore) Delete(string) error { return nil }

func (noStore) Lock(context.Context) (func(), error) { return func() {}, nil }

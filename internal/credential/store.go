package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

const serviceName = "taiga-cli"

var ErrNotFound = errors.New("credential not found")

type Tokens struct {
	AuthToken    string `json:"auth_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Location says where a credential is kept.
type Location struct {
	// File is the credentials file holding the credential, and empty when
	// the OS keyring holds it.
	File string
}

type Store interface {
	Get(account string) (Tokens, Location, error)
	Set(account string, tokens Tokens) (Location, error)
	Delete(account string) error
	// Lock waits until no other taiga process holds the store's lock, and
	// returns the function that releases it. A refresh runs under it, since
	// Taiga retires a refresh token once it is used and two processes
	// refreshing the same pair would each strand the other.
	Lock(ctx context.Context) (unlock func(), err error)
}

// KeyringError is an OS keyring that exists but could not be used, as
// distinct from there being none.
type KeyringError struct {
	Op  string
	Err error
	// Headless is set when this session has no desktop, so nothing could
	// have shown a prompt to unlock the keyring.
	Headless bool
}

func (e *KeyringError) Error() string { return e.Op + ": " + e.Err.Error() }

func (e *KeyringError) Unwrap() error { return e.Err }

type backend interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Delete(service, account string) error
}

type osBackend struct{}

func (osBackend) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (osBackend) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}

func (osBackend) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

// KeyringStore keeps credentials in the OS keyring. A server, a container or
// an SSH login has no desktop and so, on Linux, nothing providing the Secret
// Service; there, in auto mode, it keeps them in a file only the user can read
// instead, the way gh and docker do, because the alternative is a login that
// cannot be saved at all. A keyring that exists but is locked, or whose prompt
// was dismissed, is still an error: that person has a keyring and would not
// want the token written to disk behind their back.
type KeyringStore struct {
	backend backend
	// file is nil when there is nowhere to fall back to.
	file *fileStore
	// lockPath is empty when refreshes need not be coordinated.
	lockPath string
	// secretServiceMissing reports that no keyring service exists at all.
	secretServiceMissing func() bool
	// headless reports that no desktop is available to show a prompt.
	headless func() bool
}

func newKeyringStore(backend backend) *KeyringStore { return &KeyringStore{backend: backend} }

func Account(profile, apiURL string) string { return profile + "|" + apiURL }

func (s *KeyringStore) Get(account string) (Tokens, Location, error) {
	// The file is read first because an entry in it is always the newest
	// one: it is written only when there is no keyring, and removed whenever
	// the keyring takes the credential. The keyring, by contrast, may still
	// hold a copy from before it went away, whose refresh token has since
	// been rotated out.
	tokens, err := s.file.get(account)
	if err == nil {
		return tokens, Location{File: s.file.path}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Tokens{}, Location{}, err
	}
	value, err := s.backend.Get(serviceName, account)
	if err == nil {
		if err := json.Unmarshal([]byte(value), &tokens); err != nil {
			return Tokens{}, Location{}, fmt.Errorf("decode OS keyring entry: %w", err)
		}
		return tokens, Location{}, nil
	}
	if errors.Is(err, keyring.ErrNotFound) || s.keyringMissing(err) {
		return Tokens{}, Location{}, ErrNotFound
	}
	return Tokens{}, Location{}, s.keyringError("read OS keyring", err)
}

func (s *KeyringStore) Set(account string, tokens Tokens) (Location, error) {
	data, err := json.Marshal(tokens)
	if err != nil {
		return Location{}, fmt.Errorf("encode credential: %w", err)
	}
	err = s.backend.Set(serviceName, account, string(data))
	if err == nil {
		// Once the keyring holds the credential, a copy left in the file is
		// only a readable token that nothing uses.
		return Location{}, s.file.delete(account)
	}
	if !s.keyringMissing(err) {
		return Location{}, s.keyringError("write OS keyring", err)
	}
	if err := s.file.set(account, tokens); err != nil {
		return Location{}, err
	}
	return Location{File: s.file.path}, nil
}

func (s *KeyringStore) Delete(account string) error {
	err := s.backend.Delete(serviceName, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) && !s.keyringMissing(err) {
		return s.keyringError("delete OS keyring entry", err)
	}
	return s.file.delete(account)
}

func (s *KeyringStore) Lock(ctx context.Context) (func(), error) {
	return lockFile(ctx, s.lockPath)
}

// keyringMissing reports whether err came from there being no keyring to
// use, as opposed to a keyring that refused. Without a file to fall back to,
// the difference does not matter and every failure is reported.
func (s *KeyringStore) keyringMissing(err error) bool {
	if s.file == nil {
		return false
	}
	return errors.Is(err, keyring.ErrUnsupportedPlatform) || s.secretServiceMissing()
}

func (s *KeyringStore) keyringError(op string, err error) error {
	return &KeyringError{Op: op, Err: err, Headless: s.headless != nil && s.headless()}
}

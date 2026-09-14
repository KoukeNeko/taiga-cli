package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/KoukeNeko/taiga-cli/internal/atomicfile"
	keyring "github.com/zalando/go-keyring"
)

const serviceName = "taiga-cli"

// FileName is the credentials file kept beside the config file for a machine
// that has no OS keyring to hold a credential.
const FileName = "credentials.json"

var ErrNotFound = errors.New("credential not found")

type Tokens struct {
	AuthToken    string `json:"auth_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Saved says where Set put a credential.
type Saved struct {
	// File is the credentials file the credential went to because there was
	// no OS keyring to take it, and empty when the keyring took it.
	File string
}

type Store interface {
	Get(account string) (Tokens, error)
	Set(account string, tokens Tokens) (Saved, error)
	Delete(account string) error
}

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
// Service; there it keeps them in a file only the user can read instead, the
// way gh and docker do, because the alternative is a login that cannot be
// saved at all. A keyring that exists but is locked, or whose prompt was
// dismissed, is still an error: that person has a keyring and would not want
// the token written to disk behind their back.
type KeyringStore struct {
	backend backend
	// file is nil when there is nowhere to fall back to.
	file *fileStore
	// secretServiceMissing reports that no keyring service exists at all.
	secretServiceMissing func() bool
}

func NewKeyringStore(filePath string) *KeyringStore {
	return &KeyringStore{backend: osBackend{}, file: &fileStore{path: filePath}, secretServiceMissing: secretServiceMissing}
}

func newKeyringStore(backend backend) *KeyringStore { return &KeyringStore{backend: backend} }

func Account(profile, apiURL string) string { return profile + "|" + apiURL }

func (s *KeyringStore) Get(account string) (Tokens, error) {
	// The file is read first because an entry in it is always the newest
	// one: it is written only when there is no keyring, and removed whenever
	// the keyring takes the credential. The keyring, by contrast, may still
	// hold a copy from before it went away, whose refresh token has since
	// been rotated out.
	tokens, err := s.file.get(account)
	if !errors.Is(err, ErrNotFound) {
		return tokens, err
	}
	value, err := s.backend.Get(serviceName, account)
	if err == nil {
		if err := json.Unmarshal([]byte(value), &tokens); err != nil {
			return Tokens{}, fmt.Errorf("decode OS keyring entry: %w", err)
		}
		return tokens, nil
	}
	if errors.Is(err, keyring.ErrNotFound) || s.keyringMissing(err) {
		return Tokens{}, ErrNotFound
	}
	return Tokens{}, fmt.Errorf("read OS keyring: %w", err)
}

func (s *KeyringStore) Set(account string, tokens Tokens) (Saved, error) {
	data, err := json.Marshal(tokens)
	if err != nil {
		return Saved{}, fmt.Errorf("encode credential: %w", err)
	}
	err = s.backend.Set(serviceName, account, string(data))
	if err == nil {
		// Once the keyring holds the credential, a copy left in the file is
		// only a readable token that nothing uses.
		return Saved{}, s.file.delete(account)
	}
	if !s.keyringMissing(err) {
		return Saved{}, fmt.Errorf("write OS keyring: %w", err)
	}
	if err := s.file.set(account, tokens); err != nil {
		return Saved{}, err
	}
	return Saved{File: s.file.path}, nil
}

func (s *KeyringStore) Delete(account string) error {
	err := s.backend.Delete(serviceName, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) && !s.keyringMissing(err) {
		return fmt.Errorf("delete OS keyring entry: %w", err)
	}
	return s.file.delete(account)
}

// keyringMissing reports whether err came from there being no keyring to
// use, as opposed to a keyring that refused.
func (s *KeyringStore) keyringMissing(err error) bool {
	if s.file == nil {
		return false
	}
	return errors.Is(err, keyring.ErrUnsupportedPlatform) || s.secretServiceMissing()
}

// fileStore holds credentials as a JSON object keyed by account. Its methods
// accept a nil receiver, which stands for a store with no file: nothing is
// found in it and nothing needs removing from it.
type fileStore struct {
	path string
}

func (f *fileStore) get(account string) (Tokens, error) {
	if f == nil {
		return Tokens{}, ErrNotFound
	}
	entries, err := f.load()
	if err != nil {
		return Tokens{}, err
	}
	tokens, ok := entries[account]
	if !ok {
		return Tokens{}, ErrNotFound
	}
	return tokens, nil
}

func (f *fileStore) set(account string, tokens Tokens) error {
	entries, err := f.load()
	if err != nil {
		return err
	}
	entries[account] = tokens
	return f.save(entries)
}

func (f *fileStore) delete(account string) error {
	if f == nil {
		return nil
	}
	entries, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := entries[account]; !ok {
		return nil
	}
	delete(entries, account)
	if len(entries) > 0 {
		return f.save(entries)
	}
	if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove credentials file %q: %w", f.path, err)
	}
	return nil
}

func (f *fileStore) load() (map[string]Tokens, error) {
	entries := map[string]Tokens{}
	file, err := os.Open(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials file %q: %w", f.path, err)
	}
	defer func() { _ = file.Close() }()
	// A file others can read may already have been read, so it is refused
	// rather than tightened and used: the person should know, and decide
	// whether to log out and in again. The mode is taken from the open file
	// so that it describes what is read. Windows reports no such bits.
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("read credentials file %q: %w", f.path, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("credentials file %q can be read by other users (mode %04o); run `chmod 600 %s`, and log in again if the token may have been seen", f.path, info.Mode().Perm(), f.path)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read credentials file %q: %w", f.path, err)
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode credentials file %q: %w", f.path, err)
	}
	return entries, nil
}

func (f *fileStore) save(entries map[string]Tokens) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials file: %w", err)
	}
	if err := atomicfile.Write(f.path, append(data, '\n')); err != nil {
		return fmt.Errorf("save credentials file %q: %w", f.path, err)
	}
	return nil
}

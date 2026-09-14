package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/KoukeNeko/taiga-cli/internal/atomicfile"
)

// FileName is the credentials file kept in the configuration directory, for
// a machine with no OS keyring or a person who chose a file.
const FileName = "credentials.json"

// lockFileName is the file whose lock serializes refreshes across processes.
// It holds nothing and is left in place, since removing it would let two
// processes lock two different files of the same name.
const lockFileName = "credentials.lock"

// FileStore keeps credentials only in the credentials file, for a person who
// chose that rather than have the keyring tried first.
type FileStore struct {
	file     *fileStore
	lockPath string
}

func (s *FileStore) Get(account string) (Tokens, Location, error) {
	tokens, err := s.file.get(account)
	if err != nil {
		return Tokens{}, Location{}, err
	}
	return tokens, Location{File: s.file.path}, nil
}

func (s *FileStore) Set(account string, tokens Tokens) (Location, error) {
	if err := s.file.set(account, tokens); err != nil {
		return Location{}, err
	}
	return Location{File: s.file.path}, nil
}

func (s *FileStore) Delete(account string) error { return s.file.delete(account) }

func (s *FileStore) Lock(ctx context.Context) (func(), error) { return lockFile(ctx, s.lockPath) }

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

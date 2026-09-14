package credential

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

type fakeBackend struct {
	values    map[string]string
	getErr    error
	setErr    error
	deleteErr error
}

func (f *fakeBackend) key(service, account string) string { return service + "|" + account }

func (f *fakeBackend) Get(service, account string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	value, ok := f.values[f.key(service, account)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeBackend) Set(service, account, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.values[f.key(service, account)] = value
	return nil
}

func (f *fakeBackend) Delete(service, account string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	key := f.key(service, account)
	if _, ok := f.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, key)
	return nil
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	backend := &fakeBackend{values: map[string]string{}}
	store := newKeyringStore(backend)
	account := Account("local", "https://example.test/api/v1/")
	want := Tokens{AuthToken: "auth", RefreshToken: "refresh"}
	if saved, err := store.Set(account, want); err != nil || saved.File != "" {
		t.Fatalf("Set() = %#v, %v", saved, err)
	}
	got, err := store.Get(account)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
	if err := store.Delete(account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestKeyringStoreDeleteMissingIsIdempotent(t *testing.T) {
	store := newKeyringStore(&fakeBackend{values: map[string]string{}})
	if err := store.Delete("missing"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestKeyringStoreRejectsMalformedEntry(t *testing.T) {
	backend := &fakeBackend{values: map[string]string{serviceName + "|broken": "not-json"}}
	_, err := newKeyringStore(backend).Get("broken")
	if err == nil || !strings.Contains(err.Error(), "decode OS keyring entry") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestKeyringStoreWrapsBackendErrors(t *testing.T) {
	backendErr := errors.New("backend unavailable")
	tests := []struct {
		name string
		call func(*KeyringStore) error
		want string
		fake *fakeBackend
	}{
		{name: "get", fake: &fakeBackend{values: map[string]string{}, getErr: backendErr}, call: func(store *KeyringStore) error { _, err := store.Get("account"); return err }, want: "read OS keyring"},
		{name: "set", fake: &fakeBackend{values: map[string]string{}, setErr: backendErr}, call: func(store *KeyringStore) error { _, err := store.Set("account", Tokens{AuthToken: "auth"}); return err }, want: "write OS keyring"},
		{name: "delete", fake: &fakeBackend{values: map[string]string{}, deleteErr: backendErr}, call: func(store *KeyringStore) error { return store.Delete("account") }, want: "delete OS keyring"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call(newKeyringStore(test.fake))
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, backendErr) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGetReadsAStoredCredential(t *testing.T) {
	backend := &fakeBackend{values: map[string]string{
		serviceName + "|account": `{"auth_token":"current"}`,
	}}
	if tokens, err := newKeyringStore(backend).Get("account"); err != nil || tokens.AuthToken != "current" {
		t.Fatalf("Get() = %#v, %v", tokens, err)
	}
}

func TestGetReportsNotFoundWhenMissing(t *testing.T) {
	if _, err := newKeyringStore(&fakeBackend{values: map[string]string{}}).Get("account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesTheCredential(t *testing.T) {
	backend := &fakeBackend{values: map[string]string{
		serviceName + "|account": `{"auth_token":"current"}`,
	}}
	if err := newKeyringStore(backend).Delete("account"); err != nil {
		t.Fatal(err)
	}
	if len(backend.values) != 0 {
		t.Fatalf("logout left credentials behind: %#v", backend.values)
	}
}

// noKeyringStore is the store on a machine with no Secret Service: every
// keyring call fails, and the bus confirms nothing provides the service.
func noKeyringStore(t *testing.T) (*KeyringStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	unavailable := errors.New("The name org.freedesktop.secrets was not provided by any .service files")
	backend := &fakeBackend{values: map[string]string{}, getErr: unavailable, setErr: unavailable, deleteErr: unavailable}
	return &KeyringStore{backend: backend, file: &fileStore{path: path}, secretServiceMissing: func() bool { return true }}, path
}

func TestWithoutAKeyringTheCredentialGoesToAPrivateFile(t *testing.T) {
	store, path := noKeyringStore(t)
	account := Account("default", "https://example.test/api/v1/")
	want := Tokens{AuthToken: "auth", RefreshToken: "refresh"}
	if _, err := store.Get(account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() before login = %v, want ErrNotFound", err)
	}
	saved, err := store.Set(account, want)
	if err != nil || saved.File != path {
		t.Fatalf("Set() = %#v, %v, want file %q", saved, err, path)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("credentials file mode = %o, want 600", mode)
		}
	}
	if got, err := store.Get(account); err != nil || got != want {
		t.Fatalf("Get() = %#v, %v, want %#v", got, err, want)
	}
	if err := store.Delete(account); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout of the last account left the file behind: %v", err)
	}
}

func TestDeletingOneAccountKeepsTheOthersInTheFile(t *testing.T) {
	store, _ := noKeyringStore(t)
	for _, account := range []string{"first", "second"} {
		if _, err := store.Set(account, Tokens{AuthToken: account}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete("first"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get("second"); err != nil || got.AuthToken != "second" {
		t.Fatalf("Get(second) = %#v, %v", got, err)
	}
	if _, err := store.Get("first"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(first) = %v, want ErrNotFound", err)
	}
}

// A keyring that is present but refuses, being locked or having its prompt
// dismissed, is reported rather than worked around with a file.
func TestARefusingKeyringIsNotReplacedByTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	locked := errors.New("Cannot create an item in a locked collection")
	backend := &fakeBackend{values: map[string]string{}, getErr: locked, setErr: locked}
	store := &KeyringStore{backend: backend, file: &fileStore{path: path}, secretServiceMissing: func() bool { return false }}
	if _, err := store.Set("account", Tokens{AuthToken: "auth"}); !errors.Is(err, locked) {
		t.Fatalf("Set() = %v, want the keyring's error", err)
	}
	if _, err := store.Get("account"); !errors.Is(err, locked) {
		t.Fatalf("Get() = %v, want the keyring's error", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refusing keyring still produced a credentials file: %v", err)
	}
}

func TestAnUnsupportedPlatformUsesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	backend := &fakeBackend{values: map[string]string{}, setErr: keyring.ErrUnsupportedPlatform}
	store := &KeyringStore{backend: backend, file: &fileStore{path: path}, secretServiceMissing: func() bool { return false }}
	if saved, err := store.Set("account", Tokens{AuthToken: "auth"}); err != nil || saved.File != path {
		t.Fatalf("Set() = %#v, %v", saved, err)
	}
}

// A credential saved to the file before a keyring was installed keeps
// working, and moves out of the file the next time it is written.
func TestAKeyringArrivingLaterTakesOverFromTheFile(t *testing.T) {
	store, path := noKeyringStore(t)
	if _, err := store.Set("account", Tokens{AuthToken: "old"}); err != nil {
		t.Fatal(err)
	}
	keyringNow := &fakeBackend{values: map[string]string{}}
	store.backend = keyringNow
	store.secretServiceMissing = func() bool { return false }
	if got, err := store.Get("account"); err != nil || got.AuthToken != "old" {
		t.Fatalf("Get() = %#v, %v, want the file's credential", got, err)
	}
	if saved, err := store.Set("account", Tokens{AuthToken: "new"}); err != nil || saved.File != "" {
		t.Fatalf("Set() = %#v, %v, want the keyring", saved, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file kept a token the keyring now holds: %v", err)
	}
	if got, err := store.Get("account"); err != nil || got.AuthToken != "new" {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
}

func TestAMalformedCredentialsFileIsReported(t *testing.T) {
	store, path := noKeyringStore(t)
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("account"); err == nil || !strings.Contains(err.Error(), "decode credentials file") {
		t.Fatalf("Get() = %v", err)
	}
}

// A keyring that comes back may still hold the credential from before it
// went away. The file's newer credential wins, since its refresh token is the
// one the server still honours.
func TestANewerFileCredentialWinsOverAStaleKeyringCopy(t *testing.T) {
	store, _ := noKeyringStore(t)
	if _, err := store.Set("account", Tokens{AuthToken: "newer", RefreshToken: "current"}); err != nil {
		t.Fatal(err)
	}
	store.backend = &fakeBackend{values: map[string]string{
		serviceName + "|account": `{"auth_token":"stale","refresh_token":"rotated-out"}`,
	}}
	store.secretServiceMissing = func() bool { return false }
	if got, err := store.Get("account"); err != nil || got.AuthToken != "newer" {
		t.Fatalf("Get() = %#v, %v, want the file's newer credential", got, err)
	}
}

func TestACredentialsFileOthersCanReadIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports no permission bits")
	}
	store, path := noKeyringStore(t)
	if _, err := store.Set("account", Tokens{AuthToken: "auth"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := store.Get("account")
	if err == nil || !strings.Contains(err.Error(), "can be read by other users") || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("Get() = %v", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("the refused file was changed: %v, %v", info.Mode(), statErr)
	}
}

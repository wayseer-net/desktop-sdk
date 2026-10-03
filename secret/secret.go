// Package secret reads secrets from the platform's keyring: the Secret Service on Linux, the
// Keychain on macOS and the Credential Manager on Windows. It never writes to one.
package secret

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Store is a keyring, read by service and account.
type Store interface {
	Get(service, account string) (string, error)
}

var (
	// ErrNotFound is a store's answer when it holds nothing for the service and account.
	ErrNotFound = errors.New("not found")
	// ErrNoKeyring is a store's answer when no keyring is running to ask.
	ErrNoKeyring = errors.New("no keyring")
)

// NoKeyring is ErrNoKeyring with why.
func NoKeyring(why string) error { return fmt.Errorf("%w: %s", ErrNoKeyring, why) }

// Ref names a keyring entry; it is safe to log.
type Ref struct{ Service, Account string }

// ParseRef reads <service>/<account>; the account may hold further slashes.
func ParseRef(s string) (Ref, error) {
	service, account, _ := strings.Cut(s, "/")
	if service == "" || account == "" || strings.TrimSpace(s) != s {
		return Ref{}, fmt.Errorf("%s is not <service>/<account>", s)
	}
	return Ref{service, account}, nil
}

var (
	mu    sync.Mutex
	store Store
)

// current is the store in use: the platform's, unless a test has set one.
func current() Store {
	mu.Lock()
	defer mu.Unlock()
	if store == nil {
		store = platform()
	}
	return store
}

// Use makes s the store Lookup reads, for tests; restore puts back the one before.
func Use(s Store) (restore func()) {
	mu.Lock()
	defer mu.Unlock()
	was := store
	store = s
	return func() {
		mu.Lock()
		defer mu.Unlock()
		store = was
	}
}

// Lookup reads the secret ref names. Errors name the service and account, never the secret.
func Lookup(ref string) (string, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return "", fmt.Errorf("secret_keyring: %w", err)
	}
	v, err := current().Get(r.Service, r.Account)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", fmt.Errorf("secret_keyring: nothing is stored for service %q, account %q", r.Service, r.Account)
	case errors.Is(err, ErrNoKeyring):
		return "", fmt.Errorf("secret_keyring: %w; use secret_file or secret_env instead", err)
	case err != nil:
		return "", fmt.Errorf("secret_keyring: service %q, account %q: %w", r.Service, r.Account, err)
	}
	return v, nil
}

// CheckSources reports whether a secret's source is named, refusing more than one of a file,
// an environment variable and a keyring entry, and a keyring entry not <service>/<account>.
func CheckSources(file, env, keyring string) (named bool, err error) {
	n := 0
	for _, s := range []string{file, env, keyring} {
		if s != "" {
			n++
		}
	}
	if n > 1 {
		return true, errors.New("set only one of secret_file, secret_env or secret_keyring")
	}
	if keyring != "" {
		if _, err := ParseRef(keyring); err != nil {
			return true, fmt.Errorf("secret_keyring: %w", err)
		}
	}
	return n == 1, nil
}

// Map is a store holding secrets by <service>/<account>, for tests.
type Map map[string]string

// Get reads the secret held for service and account.
func (m Map) Get(service, account string) (string, error) {
	if v, ok := m[service+"/"+account]; ok {
		return v, nil
	}
	return "", ErrNotFound
}

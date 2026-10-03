package secret_test

import (
	"errors"
	"strings"
	"testing"

	"wayseer.dev/sdk/secret"
)

// canary is the secret the fake keyring holds, so finding it in any error is a leak.
const canary = "canary-5e1c-k3y"

// fake is a keyring holding secrets by service and account.
type fake struct {
	items map[[2]string]string
	err   error
}

func (f fake) Get(service, account string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.items[[2]string{service, account}]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func holding(t *testing.T, f fake) {
	t.Helper()
	t.Cleanup(secret.Use(f))
}

func TestKeyringRefIsServiceSlashAccount(t *testing.T) {
	for in, want := range map[string]secret.Ref{
		"wayseer/anthropic":     {Service: "wayseer", Account: "anthropic"},
		"wayseer/ops@db-01/dsn": {Service: "wayseer", Account: "ops@db-01/dsn"},
	} {
		if got, err := secret.ParseRef(in); err != nil || got != want {
			t.Errorf("ParseRef(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "wayseer", "/anthropic", "wayseer/", " wayseer/anthropic"} {
		if _, err := secret.ParseRef(in); err == nil || !strings.Contains(err.Error(), "<service>/<account>") {
			t.Errorf("ParseRef(%q) = %v; want a refusal naming the form", in, err)
		}
	}
}

func TestKeyringLookupReadsTheStore(t *testing.T) {
	holding(t, fake{items: map[[2]string]string{{"wayseer", "anthropic"}: canary}})
	if v, err := secret.Lookup("wayseer/anthropic"); err != nil || v != canary {
		t.Errorf("Lookup = %q, %v", v, err)
	}
}

func TestKeyringErrorsNameServiceAndAccountNeverTheSecret(t *testing.T) {
	holding(t, fake{items: map[[2]string]string{{"wayseer", "anthropic"}: canary}})
	for ref, want := range map[string]string{
		"wayseer/zen":     `secret_keyring: nothing is stored for service "wayseer", account "zen"`,
		"other/anthropic": `secret_keyring: nothing is stored for service "other", account "anthropic"`,
		"wayseer":         "secret_keyring: wayseer is not <service>/<account>",
	} {
		_, err := secret.Lookup(ref)
		if err == nil || err.Error() != want || strings.Contains(err.Error(), canary) {
			t.Errorf("Lookup(%q) = %v; want %q", ref, err, want)
		}
	}
}

func TestKeyringStoreFailuresNameWhereNeverWhat(t *testing.T) {
	holding(t, fake{err: errors.New("the keyring is locked")})
	_, err := secret.Lookup("wayseer/anthropic")
	want := `secret_keyring: service "wayseer", account "anthropic": the keyring is locked`
	if err == nil || err.Error() != want {
		t.Errorf("Lookup = %v; want %q", err, want)
	}
}

func TestNoKeyringNamesTheOtherOptions(t *testing.T) {
	holding(t, fake{err: secret.NoKeyring("the Secret Service is not running")})
	_, err := secret.Lookup("wayseer/anthropic")
	if !errors.Is(err, secret.ErrNoKeyring) {
		t.Fatalf("Lookup = %v; want ErrNoKeyring", err)
	}
	want := "secret_keyring: no keyring: the Secret Service is not running; use secret_file or secret_env instead"
	if err.Error() != want {
		t.Errorf("Lookup = %q; want %q", err, want)
	}
}

func TestAtMostOneSecretSourceIsNamed(t *testing.T) {
	for _, tc := range []struct {
		file, env, keyring string
		named              bool
		err                string
	}{
		{"", "", "", false, ""},
		{"~/k", "", "", true, ""},
		{"", "K", "", true, ""},
		{"", "", "wayseer/k", true, ""},
		{"~/k", "K", "", true, "set only one of secret_file, secret_env or secret_keyring"},
		{"", "K", "wayseer/k", true, "set only one of secret_file, secret_env or secret_keyring"},
		{"", "", "wayseer", true, "secret_keyring: wayseer is not <service>/<account>"},
	} {
		named, err := secret.CheckSources(tc.file, tc.env, tc.keyring)
		if named != tc.named || (err == nil) != (tc.err == "") || err != nil && err.Error() != tc.err {
			t.Errorf("%q %q %q: %v, %v; want %v, %q", tc.file, tc.env, tc.keyring, named, err, tc.named, tc.err)
		}
	}
}

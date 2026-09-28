package sdktest

import (
	"mindseye/internal/secret"
	"testing"
)

// Keyring makes the keyring hold entries, keyed <service>/<account>, for the rest of the test,
// so a module's secret_keyring can be tested without the platform's keyring.
func Keyring(t testing.TB, entries map[string]string) {
	t.Helper()
	t.Cleanup(secret.Use(secret.Map(entries)))
}

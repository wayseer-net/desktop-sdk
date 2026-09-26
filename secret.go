package sdk

import (
	"errors"
	"fmt"
	"mindseye/internal/kernel"
	"os"
	"path/filepath"
	"strings"
)

// Secret is a credential; every way of printing it shows [redacted]. Reveal gives the value, for
// handing to a client library only.
type Secret = kernel.Secret

// SecretOptions name where a secret is kept. Embed them in a module's options with
// `yaml:",inline"` so the config reads secret_file: or secret_env:.
type SecretOptions struct {
	SecretFile string `yaml:"secret_file"` // a file holding the secret; ~/ is the home directory
	SecretEnv  string `yaml:"secret_env"`  // or the environment variable holding it
}

// Set reports whether the options name a secret.
func (o SecretOptions) Set() bool { return o.SecretFile != "" || o.SecretEnv != "" }

// Read loads the secret, trimmed of spaces, or nothing when none is named. Errors name where
// it was sought, never what was read.
func (o SecretOptions) Read() (Secret, error) {
	var s string
	switch {
	case o.SecretFile != "" && o.SecretEnv != "":
		return Secret{}, errors.New("set one of secret_file or secret_env, not both")
	case o.SecretEnv != "":
		v, ok := os.LookupEnv(o.SecretEnv)
		if !ok {
			return Secret{}, fmt.Errorf("secret_env: %s is not set", o.SecretEnv)
		}
		if s = strings.TrimSpace(v); s == "" {
			return Secret{}, fmt.Errorf("secret_env: %s is empty", o.SecretEnv)
		}
	case o.SecretFile != "":
		b, err := os.ReadFile(expandHome(o.SecretFile))
		if err != nil {
			return Secret{}, fmt.Errorf("secret_file: %w", pathOnly(err))
		}
		if s = strings.TrimSpace(string(b)); s == "" {
			return Secret{}, fmt.Errorf("secret_file: %s is empty", o.SecretFile)
		}
	default:
		return Secret{}, nil
	}
	return kernel.NewSecret(s), nil
}

// pathOnly keeps a file error's operation, path and cause, which hold nothing read from the file.
func pathOnly(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe
	}
	return errors.New("cannot read it")
}

// expandHome replaces a leading ~/ with the user's home directory.
func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return p
}

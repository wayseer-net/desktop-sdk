package sdk_test

import (
	"fmt"
	"mindseye/pkg/sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const token = "s3cr3t-t0ken"

func TestSecretOptionsReadFromEnvOrFile(t *testing.T) {
	t.Setenv("SDK_TOKEN", " "+token+"\n")
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, o := range []sdk.SecretOptions{{SecretEnv: "SDK_TOKEN"}, {SecretFile: file}} {
		s, err := o.Read()
		if err != nil || s.Reveal() != token {
			t.Errorf("%+v reads %q, %v; want the token, trimmed", o, s.Reveal(), err)
		}
		if got := fmt.Sprintf("%v %s %#v", s, s, s); strings.Contains(got, token) {
			t.Errorf("a secret formats as %q", got)
		}
	}
}

func TestSecretOptionsErrorsNameWhereNeverWhat(t *testing.T) {
	t.Setenv("SDK_EMPTY", "  ")
	t.Setenv("SDK_TOKEN", token)
	for _, tc := range []struct {
		o    sdk.SecretOptions
		want string
	}{
		{sdk.SecretOptions{SecretEnv: "SDK_UNSET_VAR"}, "secret_env: SDK_UNSET_VAR is not set"},
		{sdk.SecretOptions{SecretEnv: "SDK_EMPTY"}, "secret_env: SDK_EMPTY is empty"},
		{sdk.SecretOptions{SecretFile: "/no/such/secret"}, "secret_file: open /no/such/secret"},
		{sdk.SecretOptions{SecretEnv: "SDK_TOKEN", SecretFile: "/x"}, "one of secret_file or secret_env, not both"},
	} {
		_, err := tc.o.Read()
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), token) {
			t.Errorf("%+v: %v; want %q, without the secret", tc.o, err, tc.want)
		}
	}
}

func TestSecretOptionsUnsetReadsNothing(t *testing.T) {
	var o sdk.SecretOptions
	if s, err := o.Read(); o.Set() || err != nil || s.Reveal() != "" {
		t.Errorf("no secret named: set %v, %q, %v", o.Set(), s.Reveal(), err)
	}
}

package secret_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"wayseer.dev/sdk/secret"

	"go.yaml.in/yaml/v3"
)

func TestSecretReveals(t *testing.T) {
	if got := secret.New(canary).Reveal(); got != canary {
		t.Errorf("Reveal = %q; want the value", got)
	}
	if got := (secret.Secret{}).Reveal(); got != "" {
		t.Errorf("zero Secret reveals %q; want nothing", got)
	}
}

func TestSecretNeverShown(t *testing.T) {
	s := secret.New(canary)
	type holder struct {
		Public  secret.Secret
		private secret.Secret
	}
	h := holder{s, s}
	var buf bytes.Buffer
	for _, lg := range []*slog.Logger{
		slog.New(slog.NewTextHandler(&buf, nil)), slog.New(slog.NewJSONHandler(&buf, nil)),
	} {
		lg.Info("resolved", "token", s, slog.Any("holder", h), slog.Any("ptr", &h))
	}
	fmt.Fprintf(&buf, "%v %+v %#v %s %q %v %+v %#v", s, s, s, s, s, h, h, h)
	j, _ := json.Marshal(h)
	buf.Write(j)
	y, _ := yaml.Marshal(h)
	buf.Write(y)
	if strings.Contains(buf.String(), canary) {
		t.Fatalf("secret leaked:\n%s", buf.String())
	}
}

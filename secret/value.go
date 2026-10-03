package secret

import (
	"fmt"
	"log/slog"
)

const redacted = "[redacted]"

// Secret holds a resolved credential. Every formatting path prints [redacted]; the value sits
// behind a pointer so even reflection-based printing of an unexported field shows an address.
type Secret struct{ v *string }

// New wraps a credential read by other means, such as a module's own options.
func New(v string) Secret { return Secret{&v} }

// Reveal returns the value, for handing to a client library only.
func (s Secret) Reveal() string {
	if s.v == nil {
		return ""
	}
	return *s.v
}

// String returns [redacted].
func (Secret) String() string { return redacted }

// Format prints [redacted] for every fmt verb, including %#v.
func (Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }

// LogValue keeps slog from printing the value.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalText covers JSON and other text encoders.
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// MarshalYAML keeps the value out of any YAML we write.
func (Secret) MarshalYAML() (any, error) { return redacted, nil }

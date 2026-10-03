package model

import (
	"errors"
	"fmt"
	"strings"
)

// EntityRef identifies an entity globally and stably: <instance>/<kind>/<native-id>.
// Slashes and percent signs inside the kind and native id are percent-escaped.
type EntityRef string

var errRef = errors.New("invalid entity ref")

// NewEntityRef builds the canonical ref for a native id in a module instance.
func NewEntityRef(instance string, kind Kind, native string) (EntityRef, error) {
	if err := validRefParts(instance, kind, native); err != nil {
		return "", err
	}
	return EntityRef(instance + "/" + escape(string(kind)) + "/" + escape(native)), nil
}

// ParseEntityRef splits a canonical ref; non-canonical escapes are rejected so equal things have equal refs.
func ParseEntityRef(s string) (instance string, kind Kind, native string, err error) {
	if !strings.Contains(s, "%") {
		return parsePlainRef(s)
	}
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("%w %q: want 3 segments", errRef, s)
	}
	k, err1 := unescape(parts[1])
	n, err2 := unescape(parts[2])
	if err := errors.Join(err1, err2); err != nil {
		return "", "", "", fmt.Errorf("%w %q: %w", errRef, s, err)
	}
	ref, err := NewEntityRef(parts[0], Kind(k), n)
	if err != nil {
		return "", "", "", err
	}
	if string(ref) != s {
		return "", "", "", fmt.Errorf("%w %q: not canonical (want %q)", errRef, s, ref)
	}
	return parts[0], Kind(k), n, nil
}

// parsePlainRef handles refs without escapes, which are canonical whenever they are valid.
func parsePlainRef(s string) (instance string, kind Kind, native string, err error) {
	inst, rest, ok1 := strings.Cut(s, "/")
	k, n, ok2 := strings.Cut(rest, "/")
	if !ok1 || !ok2 || strings.Contains(n, "/") {
		return "", "", "", fmt.Errorf("%w %q: want 3 segments", errRef, s)
	}
	if err := validRefParts(inst, Kind(k), n); err != nil {
		return "", "", "", err
	}
	return inst, Kind(k), n, nil
}

// Instance is the module instance that owns the entity; empty if the ref is invalid.
func (r EntityRef) Instance() string { i, _, _, _ := ParseEntityRef(string(r)); return i }

// Module is the instance the ref names, read without checking the rest of the ref.
func (r EntityRef) Module() ModuleID { i, _, _ := strings.Cut(string(r), "/"); return ModuleID(i) }

// Kind is the entity's kind; empty if the ref is invalid.
func (r EntityRef) Kind() Kind { _, k, _, _ := ParseEntityRef(string(r)); return k }

// Native is the module's own id for the entity; empty if the ref is invalid.
func (r EntityRef) Native() string { _, _, n, _ := ParseEntityRef(string(r)); return n }

// Validate reports whether r is canonical.
func (r EntityRef) Validate() error { _, _, _, err := ParseEntityRef(string(r)); return err }

func validRefParts(instance string, kind Kind, native string) error {
	if err := validInstance(instance); err != nil {
		return err
	}
	if err := kind.Validate(); err != nil {
		return err
	}
	if native == "" {
		return fmt.Errorf("%w: empty native id", errRef)
	}
	return nil
}

// validInstance allows the names config gives module instances: lowercase, digits, '-', '_', '.'.
func validInstance(s string) error {
	if s == "" || strings.IndexFunc(s, func(c rune) bool { return !isNameRune(c) && c != '.' }) >= 0 {
		return fmt.Errorf("%w: bad instance name %q", errRef, s)
	}
	return nil
}

func isNameRune(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

var escaper = strings.NewReplacer("%", "%25", "/", "%2F")

func escape(s string) string { return escaper.Replace(s) }

// unescape decodes only %25 and %2F, the two escapes escape produces.
func unescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		switch {
		case strings.HasPrefix(s[i:], "%25"):
			b.WriteByte('%')
		case strings.HasPrefix(s[i:], "%2F"):
			b.WriteByte('/')
		default:
			return "", fmt.Errorf("bad escape at byte %d", i)
		}
		i += 2
	}
	return b.String(), nil
}

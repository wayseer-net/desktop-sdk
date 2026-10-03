package manifest

import (
	"slices"
	"strings"
)

const (
	minNamespace = 2
	maxNamespace = 32
	maxActionID  = 64
	maxKindName  = 64
)

// reservedNamespaces are first-party: every built-in module's kind prefix (plan §4.4).
var reservedNamespaces = []string{"k8s", "wayseer", "localhost", "prometheus", "sql", "proxmox"}

// coreKinds mirror the model package's core vocabulary; a test there keeps the two equal.
var coreKinds = []string{
	"alert", "cluster", "container", "database", "disk", "host", "interface", "node", "person",
	"pod", "process", "queue", "repo", "service", "table", "team",
}

// ValidNamespace reports whether ns has a namespace's shape: a lower-case letter, then
// lower-case letters, digits and '-', 2 to 32 long. Reserved ones pass; see Reserved.
func ValidNamespace(ns string) bool {
	return len(ns) >= minNamespace && len(ns) <= maxNamespace && ns[0] >= 'a' && ns[0] <= 'z' &&
		!strings.ContainsFunc(ns, func(c rune) bool { return !lower(c) && !digit(c) && c != '-' })
}

// Reserved reports whether ns belongs to a first-party module.
func Reserved(ns string) bool { return slices.Contains(reservedNamespaces, ns) }

// ReservedNamespaces returns the first-party namespaces.
func ReservedNamespaces() []string { return slices.Clone(reservedNamespaces) }

// CoreKinds returns the core kinds' names, sorted.
func CoreKinds() []string { return slices.Clone(coreKinds) }

// IsCoreKind reports whether k is a core kind.
func IsCoreKind(k string) bool { return slices.Contains(coreKinds, k) }

// ValidActionID reports whether id can name an action (ADR-0008): a lower-case letter, then
// lower-case letters, digits and dashes, at most 64 long.
func ValidActionID(id string) bool {
	return id != "" && len(id) <= maxActionID && lower(rune(id[0])) &&
		!strings.ContainsFunc(id, func(c rune) bool { return !lower(c) && !digit(c) && c != '-' })
}

// inNamespace reports whether k is "<ns>/<name>", the name being lower-case letters, digits,
// '-' and '_'.
func inNamespace(k, ns string) bool {
	prefix, name, ok := strings.Cut(k, "/")
	return ok && prefix == ns && name != "" && len(name) <= maxKindName &&
		!strings.ContainsFunc(name, func(c rune) bool { return !lower(c) && !digit(c) && c != '-' && c != '_' })
}

func lower(c rune) bool { return c >= 'a' && c <= 'z' }
func digit(c rune) bool { return c >= '0' && c <= '9' }

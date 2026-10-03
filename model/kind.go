package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Kind classifies an entity; modules use a core kind when one fits, else "<namespace>/<name>".
type Kind string

// Core kinds, rendered with bespoke glyphs by lenses (PLAN §5.2).
const (
	KindHost      Kind = "host"
	KindService   Kind = "service"
	KindContainer Kind = "container"
	KindPod       Kind = "pod"
	KindNode      Kind = "node"
	KindCluster   Kind = "cluster"
	KindDatabase  Kind = "database"
	KindTable     Kind = "table"
	KindQueue     Kind = "queue"
	KindDisk      Kind = "disk"
	KindInterface Kind = "interface"
	KindProcess   Kind = "process"
	KindPerson    Kind = "person"
	KindTeam      Kind = "team"
	KindRepo      Kind = "repo"
	KindAlert     Kind = "alert"
)

var coreKinds = map[Kind]bool{
	KindHost: true, KindService: true, KindContainer: true, KindPod: true, KindNode: true,
	KindCluster: true, KindDatabase: true, KindTable: true, KindQueue: true, KindDisk: true,
	KindInterface: true, KindProcess: true, KindPerson: true, KindTeam: true, KindRepo: true,
	KindAlert: true,
}

// IsCore reports whether k is in the curated core vocabulary.
func (k Kind) IsCore() bool { return coreKinds[k] }

// CoreKinds are the core kinds' names, sorted.
func CoreKinds() []string {
	out := make([]string, 0, len(coreKinds))
	for k := range coreKinds {
		out = append(out, string(k))
	}
	slices.Sort(out)
	return out
}

// Namespace is the module-specific prefix of k, or "" for an unprefixed kind.
func (k Kind) Namespace() string { return namespace(string(k)) }

// Validate checks the vocabulary shape: a lowercase name with at most one namespace prefix.
func (k Kind) Validate() error { return validTerm("kind", string(k)) }

// Relation names how two entities relate, with the same vocabulary rules as Kind.
type Relation string

// Core relations.
const (
	RelRunsOn    Relation = "runs_on"
	RelDependsOn Relation = "depends_on"
	RelTalksTo   Relation = "talks_to"
	RelMemberOf  Relation = "member_of"
	RelOwns      Relation = "owns"
	RelParentOf  Relation = "parent_of"
	RelSameAs    Relation = "same_as"
)

var coreRelations = map[Relation]bool{
	RelRunsOn: true, RelDependsOn: true, RelTalksTo: true, RelMemberOf: true,
	RelOwns: true, RelParentOf: true, RelSameAs: true,
}

// IsCore reports whether r is in the curated core vocabulary.
func (r Relation) IsCore() bool { return coreRelations[r] }

// Namespace is the module-specific prefix of r, or "" for an unprefixed relation.
func (r Relation) Namespace() string { return namespace(string(r)) }

// Validate checks the vocabulary shape.
func (r Relation) Validate() error { return validTerm("relation", string(r)) }

var errTerm = errors.New("invalid vocabulary term")

func validTerm(what, s string) error {
	parts := strings.Split(s, "/")
	if len(parts) > 2 {
		return fmt.Errorf("%w: %s %q has more than one namespace", errTerm, what, s)
	}
	for _, p := range parts {
		if p == "" || strings.IndexFunc(p, func(c rune) bool { return !isNameRune(c) }) >= 0 {
			return fmt.Errorf("%w: %s %q (want lowercase letters, digits, '-', '_')", errTerm, what, s)
		}
	}
	return nil
}

func namespace(s string) string {
	ns, _, ok := strings.Cut(s, "/")
	if !ok {
		return ""
	}
	return ns
}

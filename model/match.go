package model

import (
	"errors"
	"slices"
	"strconv"
)

// Why Match finds no one entity.
var (
	ErrNoMatch   = errors.New("no entity in the world")
	ErrAmbiguous = errors.New("several entities that are not the same")
)

// Matcher finds the entity a value names in one snapshot, by its name, its native ID or the
// identity rules, as a module does to link what it reads to what other modules found.
type Matcher struct {
	world  *Snapshot
	rules  []IdentityRule
	byKind map[Kind]map[string][]EntityRef // each kind's names and identity values, built when first asked
}

// NewMatcher matches values against w under rules.
func NewMatcher(w *Snapshot, rules []IdentityRule) *Matcher {
	return &Matcher{world: w, rules: rules, byKind: map[Kind]map[string][]EntityRef{}}
}

// Match is the entity of kind that value names. Entities linked as the same count as one, and
// the least ref among them is given.
func (m *Matcher) Match(kind Kind, value string) (EntityRef, error) {
	return m.MatchExcept(kind, value, func(EntityRef) bool { return false })
}

// MatchExcept is Match leaving out the entities skip names, such as those the asker made itself.
func (m *Matcher) MatchExcept(kind Kind, value string, skip func(EntityRef) bool) (EntityRef, error) {
	idx := m.index(kind)
	var found []EntityRef
	if value != "" {
		found = append(found, idx[nameKey(value)]...)
	}
	for i, r := range m.rules {
		if r.appliesTo(kind) {
			for _, v := range r.appendNormal(nil, value) {
				found = append(found, idx[ruleKey(i, v)]...)
			}
		}
	}
	found = slices.DeleteFunc(found, skip)
	if len(found) == 0 {
		return "", ErrNoMatch
	}
	slices.Sort(found)
	found = slices.Compact(found)
	if len(found) > 1 {
		same := m.world.SameAs(found[0])
		if slices.ContainsFunc(found, func(r EntityRef) bool { return !slices.Contains(same, r) }) {
			return "", ErrAmbiguous
		}
	}
	return found[0], nil
}

// index lists kind's entities under their names, native IDs and identity values.
func (m *Matcher) index(kind Kind) map[string][]EntityRef {
	if idx, ok := m.byKind[kind]; ok {
		return idx
	}
	idx := map[string][]EntityRef{}
	for ref := range m.world.ByKind(kind) {
		e, _ := m.world.Entity(ref)
		if e.Name != "" {
			idx[nameKey(e.Name)] = append(idx[nameKey(e.Name)], ref)
		}
		if n := ref.Native(); n != e.Name {
			idx[nameKey(n)] = append(idx[nameKey(n)], ref)
		}
		for i, r := range m.rules {
			for _, v := range r.values(e) {
				idx[ruleKey(i, v)] = append(idx[ruleKey(i, v)], ref)
			}
		}
	}
	m.byKind[kind] = idx
	return idx
}

func (r IdentityRule) appliesTo(k Kind) bool { return len(r.Kinds) == 0 || slices.Contains(r.Kinds, k) }

func nameKey(name string) string     { return "\x00" + name }
func ruleKey(i int, v string) string { return strconv.Itoa(i) + "\x00" + v }

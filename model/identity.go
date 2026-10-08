package model

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// IdentitySource is the Source of the same_as edges the store derives from identity rules.
const IdentitySource ModuleID = "identity"

// IdentityMatch is how an identity rule compares values.
type IdentityMatch string

// The ways an identity rule can compare values.
const (
	MatchHostname  IdentityMatch = "hostname"  // first DNS label, ignoring port, case and domain
	MatchIP        IdentityMatch = "ip"        // parsed address, ignoring port
	MatchAttribute IdentityMatch = "attribute" // exact value
)

// NameKey in an identity rule's Keys reads the entity's Name rather than an attribute.
const NameKey = "name"

// IdentityRule links entities from different sources whose Keys share a value (§5.3).
type IdentityRule struct {
	Name  string
	Match IdentityMatch
	Kinds []Kind   // kinds the rule applies to; empty means every kind
	Keys  []string // attributes to read; NameKey reads Entity.Name
}

// DefaultIdentityRules match hosts and nodes by hostname and IP, anything by cloud instance ID, and
// processes on Wayseer's own machine by pid, such as a module's and the localhost module's.
func DefaultIdentityRules() []IdentityRule {
	machines := []Kind{KindHost, KindNode}
	return []IdentityRule{
		{Name: "hostname", Match: MatchHostname, Kinds: machines, Keys: []string{NameKey, "hostname", "instance", "address"}},
		{Name: "ip", Match: MatchIP, Kinds: machines, Keys: []string{NameKey, "ip", "instance", "address"}},
		{Name: "cloud-instance", Match: MatchAttribute, Keys: []string{"cloud.instance_id"}},
		{Name: "local-process", Match: MatchAttribute, Keys: []string{"local.pid"}},
	}
}

// Validate reports a missing name or keys, an unknown match, or a malformed kind.
func (r IdentityRule) Validate() error {
	var errs []error
	if r.Name == "" {
		errs = append(errs, errors.New("identity rule needs a name"))
	}
	switch r.Match {
	case MatchHostname, MatchIP, MatchAttribute:
	default:
		errs = append(errs, fmt.Errorf("identity rule %q: match %q (want hostname, ip or attribute)", r.Name, r.Match))
	}
	if len(r.Keys) == 0 {
		errs = append(errs, fmt.Errorf("identity rule %q needs keys", r.Name))
	}
	for _, k := range r.Kinds {
		errs = append(errs, k.Validate())
	}
	return errors.Join(errs...)
}

// values returns e's normalised values under r, or nil when r does not apply to e.
func (r IdentityRule) values(e *Entity) []string {
	if len(r.Kinds) > 0 && !slices.Contains(r.Kinds, e.Kind) {
		return nil
	}
	var out []string
	for _, k := range r.Keys {
		if k == NameKey {
			out = r.appendNormal(out, e.Name)
			continue
		}
		for _, s := range flatten(e.Attrs[k]) {
			out = r.appendNormal(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (r IdentityRule) appendNormal(out []string, s string) []string {
	var n string
	switch r.Match {
	case MatchHostname:
		n = normalHostname(s)
	case MatchIP:
		n = normalIP(s)
	default:
		n = s
	}
	if n == "" {
		return out
	}
	return append(out, n)
}

// flatten returns the non-empty strings in v, looking one level into lists.
func flatten(v Value) []string {
	if v.Type() == TypeList {
		var out []string
		for _, e := range v.List() {
			if s := e.String(); s != "" && e.Type() != TypeList {
				out = append(out, s)
			}
		}
		return out
	}
	if s := v.String(); s != "" {
		return []string{s}
	}
	return nil
}

func stripPort(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return strings.Trim(s, "[]")
}

// normalHostname is the lower-cased first label, or "" for IPs and localhost.
func normalHostname(s string) string {
	h := strings.ToLower(strings.TrimSuffix(stripPort(strings.TrimSpace(s)), "."))
	if _, err := netip.ParseAddr(h); err == nil {
		return ""
	}
	h, _, _ = strings.Cut(h, ".")
	if h == "localhost" {
		return ""
	}
	return h
}

// normalIP is the canonical address, or "" for non-addresses and ones that identify no single machine.
func normalIP(s string) string {
	a, err := netip.ParseAddr(stripPort(strings.TrimSpace(s)))
	if err != nil {
		return ""
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsUnspecified() || a.IsLinkLocalUnicast() || a.IsMulticast() {
		return ""
	}
	return a.WithZone("").String()
}

// idKey is one normalised value under one rule.
type idKey struct {
	rule int
	val  string
}

// identity is the store's writer-only index from identity values to the entities holding them.
type identity struct {
	rules   []IdentityRule
	keysOf  map[EntityRef][]idKey
	members map[idKey]map[EntityRef]ModuleID
	linked  map[EntityRef]map[EntityRef]bool // same_as edges this index created
}

func newIdentity(rules []IdentityRule) *identity {
	return &identity{
		rules: slices.Clone(rules), keysOf: map[EntityRef][]idKey{},
		members: map[idKey]map[EntityRef]ModuleID{}, linked: map[EntityRef]map[EntityRef]bool{},
	}
}

// rekey replaces r's indexed values with those of e (nil when r was removed).
func (id *identity) rekey(r EntityRef, e *Entity) {
	for _, k := range id.keysOf[r] {
		delete(id.members[k], r)
		if len(id.members[k]) == 0 {
			delete(id.members, k)
		}
	}
	delete(id.keysOf, r)
	if e == nil {
		return
	}
	var keys []idKey
	for i, rule := range id.rules {
		for _, v := range rule.values(e) {
			keys = append(keys, idKey{i, v})
		}
	}
	for _, k := range keys {
		if id.members[k] == nil {
			id.members[k] = map[EntityRef]ModuleID{}
		}
		id.members[k][r] = e.Source
	}
	if keys != nil {
		id.keysOf[r] = keys
	}
}

// peers are the entities from other sources that share any value with r.
func (id *identity) peers(r EntityRef, src ModuleID) map[EntityRef]bool {
	out := map[EntityRef]bool{}
	for _, k := range id.keysOf[r] {
		for p, psrc := range id.members[k] {
			if psrc != src {
				out[p] = true
			}
		}
	}
	return out
}

func (id *identity) setLinked(a, b EntityRef, on bool) {
	for _, pair := range [2][2]EntityRef{{a, b}, {b, a}} {
		m := id.linked[pair[0]]
		switch {
		case on && m == nil:
			id.linked[pair[0]] = map[EntityRef]bool{pair[1]: true}
		case on:
			m[pair[1]] = true
		default:
			delete(m, pair[1])
			if len(m) == 0 {
				delete(id.linked, pair[0])
			}
		}
	}
}

// sameAsKey orders the endpoints so each pair has one edge.
func sameAsKey(a, b EntityRef) EdgeKey {
	if b < a {
		a, b = b, a
	}
	return EdgeKey{From: a, To: b, Rel: RelSameAs}
}

// reconcileIdentity re-indexes every entity touched in this transaction, then fixes its same_as
// edges. Indexing all first means no link is made to an entity this transaction removed.
func (t *txn) reconcileIdentity() {
	id := t.s.identity
	if id == nil {
		return
	}
	for r := range t.ents {
		e, _ := t.next.entities.get(r)
		id.rekey(r, e)
	}
	for r := range t.ents {
		e, _ := t.next.entities.get(r)
		var want map[EntityRef]bool
		if e != nil {
			want = id.peers(r, e.Source)
		}
		for p := range id.linked[r] {
			if !want[p] {
				t.unlinkIdentity(r, p)
			}
		}
		for p := range want {
			t.linkIdentity(r, p)
		}
	}
}

func (t *txn) linkIdentity(a, b EntityRef) {
	k := sameAsKey(a, b)
	t.s.identity.setLinked(a, b, true)
	if _, ok := t.next.edges.get(k); ok {
		return // ours already, or a module's, which takes precedence
	}
	t.addEdge(&Edge{From: k.From, To: k.To, Rel: RelSameAs, Source: IdentitySource})
}

func (t *txn) unlinkIdentity(a, b EntityRef) {
	k := sameAsKey(a, b)
	t.s.identity.setLinked(a, b, false)
	if e, ok := t.next.edges.get(k); ok && e.Source == IdentitySource {
		t.removeEdge(k)
	}
}

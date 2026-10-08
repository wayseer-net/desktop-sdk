package model

import (
	"iter"
	"math/rand/v2"
	"slices"
	"testing"
)

func host(src, native, name string, attrs map[string]Value) Entity {
	return Entity{
		Ref: mustRef(src, "host", native), Kind: KindHost, Name: name,
		Attrs: attrs, Source: ModuleID(src),
	}
}

func mustRef(instance string, kind Kind, native string) EntityRef {
	r, err := NewEntityRef(instance, kind, native)
	if err != nil {
		panic(err)
	}
	return r
}

func identityStore() *Store {
	return NewStore(StoreOptions{Identity: DefaultIdentityRules()})
}

func sameAsEdges(s *Snapshot) []EdgeKey {
	var out []EdgeKey
	for e := range s.Edges() {
		if e.Rel == RelSameAs && e.Source == IdentitySource {
			out = append(out, e.Key())
		}
	}
	slices.SortFunc(out, compareEdgeKey)
	return out
}

func TestIdentityHostnameIgnoresPortAndDomain(t *testing.T) {
	s := identityStore()
	a := host("prom", "db-07:9100", "db-07:9100", nil)
	b := host("k8s", "db-07", "db-07.lan", nil)
	snap, err := s.Apply(&ChangeSet{Upserts: []Entity{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	want := []EdgeKey{sameAsKey(a.Ref, b.Ref)}
	if got := sameAsEdges(snap); !slices.Equal(got, want) {
		t.Fatalf("same_as = %v, want %v", got, want)
	}
	if !slices.Equal(snap.Diff().EdgesAdded, want) {
		t.Errorf("diff = %v", snap.Diff().EdgesAdded)
	}
}

func TestIdentityChangingAttributeUnlinks(t *testing.T) {
	s := identityStore()
	a := host("prom", "a", "a", map[string]Value{"instance": String("db-07:9100")})
	b := host("k8s", "b", "b", map[string]Value{"hostname": String("DB-07")})
	if snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b}}); len(sameAsEdges(snap)) != 1 {
		t.Fatalf("want one link, got %v", sameAsEdges(snap))
	}
	b.Attrs = map[string]Value{"hostname": String("db-08")}
	snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{b}})
	if got := sameAsEdges(snap); len(got) != 0 {
		t.Fatalf("still linked: %v", got)
	}
	if len(snap.Diff().EdgesRemoved) != 1 {
		t.Errorf("diff removed = %v", snap.Diff().EdgesRemoved)
	}
}

func TestIdentityIPAndAttributeRules(t *testing.T) {
	s := identityStore()
	a := host("prom", "a", "x", map[string]Value{"instance": String("10.0.0.5:9100")})
	b := host("k8s", "b", "y", map[string]Value{"ip": List(String("10.0.0.5"), String("fe80::1"))})
	c := host("cloud", "c", "z", map[string]Value{"cloud.instance_id": String("i-123")})
	d := Entity{
		Ref: mustRef("inv", "vm", "d"), Kind: "vm", Source: "inv",
		Attrs: map[string]Value{"cloud.instance_id": String("i-123")},
	}
	snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b, c, d}})
	want := []EdgeKey{sameAsKey(a.Ref, b.Ref), sameAsKey(c.Ref, d.Ref)}
	slices.SortFunc(want, compareEdgeKey)
	if got := sameAsEdges(snap); !slices.Equal(got, want) {
		t.Errorf("same_as = %v, want %v", got, want)
	}
}

func TestIdentityIgnoresSameSourceAndNonIdentifyingValues(t *testing.T) {
	s := identityStore()
	snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{
		host("prom", "a", "db-07", nil), host("prom", "b", "db-07.lan", nil), // same source
		host("x", "l1", "localhost:9100", nil), host("y", "l2", "localhost", nil), // not identifying
		host("x", "i1", "127.0.0.1", nil), host("y", "i2", "127.0.0.1:80", nil),
		host("x", "n1", "10.1.2.3", nil), host("y", "n2", "10.9.9.9", nil), // IPs aren't hostnames
		{Ref: mustRef("x", "process", "p"), Kind: KindProcess, Name: "db-07", Source: "x"}, // kind not in rule
	}})
	if got := sameAsEdges(snap); len(got) != 0 {
		t.Errorf("unexpected links: %v", got)
	}
}

func TestIdentityRemovalAndReinsertInOneTransaction(t *testing.T) {
	s := identityStore()
	a, b := host("prom", "a", "db-07", nil), host("k8s", "b", "db-07", nil)
	mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b}})
	snap := mustApply(t, s, &ChangeSet{Removes: []EntityRef{b.Ref}, Upserts: []Entity{b}})
	if len(sameAsEdges(snap)) != 1 {
		t.Errorf("re-inserted entity lost its link")
	}
	snap = mustApply(t, s, &ChangeSet{Removes: []EntityRef{a.Ref}})
	if len(sameAsEdges(snap)) != 0 {
		t.Errorf("removed entity kept its link")
	}
}

func TestIdentityLeavesModuleEdgesAlone(t *testing.T) {
	s := identityStore()
	a, b := host("prom", "a", "db-07", nil), host("k8s", "b", "db-08", nil)
	mod := Edge{From: a.Ref, To: b.Ref, Rel: RelSameAs, Source: "prom"}
	mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{mod}})
	b.Name = "db-09"
	snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{b}})
	if e, ok := snap.Edge(mod.Key()); !ok || e.Source != "prom" {
		t.Errorf("module edge = %+v, %v", e, ok)
	}
}

func TestIdentityRuleValidate(t *testing.T) {
	bad := []IdentityRule{
		{Match: MatchHostname, Keys: []string{"name"}},
		{Name: "x", Match: "fuzzy", Keys: []string{"name"}},
		{Name: "x", Match: MatchIP},
		{Name: "x", Match: MatchIP, Keys: []string{"ip"}, Kinds: []Kind{"Bad Kind"}},
	}
	for _, r := range bad {
		if r.Validate() == nil {
			t.Errorf("%+v should be invalid", r)
		}
	}
	for _, r := range DefaultIdentityRules() {
		if err := r.Validate(); err != nil {
			t.Errorf("default %s: %v", r.Name, err)
		}
	}
}

// TestIdentityOrderIndependent checks the links depend only on the final entities, not on
// the order of rules, of entities, or of the transactions that produced them.
func TestIdentityOrderIndependent(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 200 {
		rules := DefaultIdentityRules()
		rng.Shuffle(len(rules), func(i, j int) { rules[i], rules[j] = rules[j], rules[i] })
		s := NewStore(StoreOptions{Identity: rules})
		for range 1 + rng.IntN(6) {
			mustApply(t, s, randomIdentityChanges(rng))
		}
		snap := s.Current()
		if got, want := sameAsEdges(snap), identityOracle(snap); !slices.Equal(got, want) {
			t.Fatalf("trial %d: links = %v, want %v", trial, got, want)
		}
	}
}

func randomIdentityChanges(rng *rand.Rand) *ChangeSet {
	names := []string{"db-07", "db-07.lan", "db-07:9100", "db-08", "10.0.0.5", "10.0.0.5:80", "web"}
	var c ChangeSet
	for range rng.IntN(8) {
		src := []string{"a", "b", "c"}[rng.IntN(3)]
		native := string(rune('p' + rng.IntN(5)))
		if rng.IntN(4) == 0 {
			c.Removes = append(c.Removes, mustRef(src, "host", native))
			continue
		}
		attrs := map[string]Value{}
		if rng.IntN(2) == 0 {
			attrs["ip"] = String(names[4+rng.IntN(2)])
		}
		if rng.IntN(3) == 0 {
			attrs["cloud.instance_id"] = String([]string{"i-1", "i-2"}[rng.IntN(2)])
		}
		c.Upserts = append(c.Upserts, host(src, native, names[rng.IntN(len(names))], attrs))
	}
	return &c
}

// identityOracle links every pair from different sources that shares a value under any rule.
func identityOracle(s *Snapshot) []EdgeKey {
	var ents []*Entity
	for e := range s.Entities() {
		ents = append(ents, e)
	}
	var out []EdgeKey
	for i, a := range ents {
		for _, b := range ents[i+1:] {
			if a.Source != b.Source && shareValue(a, b) {
				out = append(out, sameAsKey(a.Ref, b.Ref))
			}
		}
	}
	slices.SortFunc(out, compareEdgeKey)
	return out
}

func shareValue(a, b *Entity) bool {
	for _, r := range DefaultIdentityRules() {
		for _, v := range r.values(a) {
			if slices.Contains(r.values(b), v) {
				return true
			}
		}
	}
	return false
}

func TestSameAsIsTheWholeGroup(t *testing.T) {
	s := identityStore()
	a := host("prom", "a", "db-07", nil)
	b := host("k8s", "b", "db-07.lan", map[string]Value{"ip": String("10.0.0.5")})
	c := host("cloud", "c", "vm-9", map[string]Value{"ip": String("10.0.0.5")})
	d := host("prom", "d", "web-01", nil)
	snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b, c, d}})
	want := []EntityRef{a.Ref, b.Ref, c.Ref}
	slices.Sort(want)
	for _, r := range want {
		if got := snap.SameAs(r); !slices.Equal(got, want) {
			t.Errorf("SameAs(%s) = %v, want %v", r, got, want)
		}
	}
	if got := snap.SameAs(d.Ref); !slices.Equal(got, []EntityRef{d.Ref}) {
		t.Errorf("an unlinked entity's group = %v", got)
	}
	if got := snap.SameAs("x/host/gone"); !slices.Equal(got, []EntityRef{"x/host/gone"}) {
		t.Errorf("a missing entity's group = %v", got)
	}
}

// TestIdentityProperties checks, over random worlds, that groups are symmetric, that no link
// joins two entities from one module, and that giving everything unique keys removes every link.
func TestIdentityProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := range 200 {
		s := identityStore()
		for range 1 + rng.IntN(6) {
			mustApply(t, s, randomIdentityChanges(rng))
		}
		snap := s.Current()
		for e := range snap.Edges() {
			a, _ := snap.Entity(e.From)
			b, _ := snap.Entity(e.To)
			if e.Source == IdentitySource && a.Source == b.Source {
				t.Fatalf("trial %d: %s links two entities from %s", trial, e.Key(), a.Source)
			}
		}
		for e := range snap.Entities() {
			for _, p := range snap.SameAs(e.Ref) {
				if !slices.Contains(snap.SameAs(p), e.Ref) {
					t.Fatalf("trial %d: %s is in %s's group but not the reverse", trial, p, e.Ref)
				}
			}
		}
		checkGroups(t, trial, snap)
		var unique ChangeSet
		for e := range snap.Entities() {
			u := *e
			u.Name, u.Attrs = "u-"+string(u.Ref), nil
			unique.Upserts = append(unique.Upserts, u)
		}
		if got := sameAsEdges(mustApply(t, s, &unique)); len(got) != 0 {
			t.Fatalf("trial %d: unique keys left links %v", trial, got)
		}
	}
}

// TestIdentityNeverLinksToAnEntityRemovedAlongside removes one entity while adding its match
// from a third source, then brings it back unmatched: its edge lists must hold only live edges.
func TestIdentityNeverLinksToAnEntityRemovedAlongside(t *testing.T) {
	for range 50 { // the store visits touched entities in map order
		s := identityStore()
		a, b := host("a", "p", "db-07", nil), host("b", "p", "db-07", nil)
		mustApply(t, s, &ChangeSet{Upserts: []Entity{a, b}})
		c := host("c", "p", "db-07", nil)
		mustApply(t, s, &ChangeSet{Removes: []EntityRef{a.Ref}, Upserts: []Entity{c}})
		a.Name = "db-08"
		snap := mustApply(t, s, &ChangeSet{Upserts: []Entity{a}})
		for e := range snap.Edges() {
			if e.From == a.Ref || e.To == a.Ref {
				t.Fatalf("unmatched entity has edge %s", e.Key())
			}
		}
		for e := range concat(snap.Out(a.Ref), snap.In(a.Ref)) {
			t.Fatalf("unmatched entity lists edge %s", e.Key())
		}
	}
}

// checkGroups compares the grouped index and GroupsOf with a scan of every same_as edge.
func checkGroups(t *testing.T, trial int, snap *Snapshot) {
	t.Helper()
	var want []EntityRef
	for e := range snap.Edges() {
		if e.Rel == RelSameAs {
			want = append(want, e.From, e.To)
		}
	}
	slices.Sort(want)
	want = slices.Compact(want)
	if got := slices.Sorted(snap.Grouped()); !slices.Equal(got, want) {
		t.Fatalf("trial %d: grouped %v, want %v", trial, got, want)
	}
	g := GroupsOf(snap)
	for _, r := range want {
		if got := slices.Sorted(slices.Values(g.Members(r))); !slices.Equal(got, snap.SameAs(r)) || g.Face(g.Face(r)) != g.Face(r) {
			t.Fatalf("trial %d: %s's members %v, want %v", trial, r, got, snap.SameAs(r))
		}
	}
}

func concat[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, s := range seqs {
			for v := range s {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func TestIdentityLinksAModuleToItsLocalProcess(t *testing.T) {
	pid := map[string]Value{"local.pid": Number(4242)}
	module := Entity{Ref: mustRef("wayseer", "wayseer/module", "aircraft"), Kind: "wayseer/module", Source: "wayseer", Attrs: pid}
	proc := Entity{Ref: mustRef("localhost", "process", "4242"), Kind: KindProcess, Source: "localhost", Attrs: pid}
	elsewhere := Entity{Ref: mustRef("prom", "process", "4242"), Kind: KindProcess, Source: "prom", Attrs: map[string]Value{"pid": Number(4242)}}
	snap := mustApply(t, identityStore(), &ChangeSet{Upserts: []Entity{module, proc, elsewhere}})
	if got, want := sameAsEdges(snap), []EdgeKey{sameAsKey(module.Ref, proc.Ref)}; !slices.Equal(got, want) {
		t.Errorf("same_as = %v, want only the module and its local process", got)
	}
}

package model

import (
	"slices"
	"testing"
)

var (
	dublin    = At(53.35, -6.26)
	singapore = At(1.35, 103.82)
)

func placedStore() *Store {
	return NewStore(StoreOptions{Places: NewGazetteer(nil, map[string]Place{"eu-west-1": dublin, "Singapore": singapore})})
}

func withAttr(e Entity, k, v string) Entity {
	e.Attrs = map[string]Value{k: String(v)}
	return e
}

func placeOf(t *testing.T, snap *Snapshot, r EntityRef) Place {
	t.Helper()
	e, ok := snap.Entity(r)
	if !ok {
		t.Fatalf("%s is missing", r)
	}
	return e.Place
}

func from(p Place, src PlaceSource) Place { p.From = src; return p }

func TestAnAttributeNamesAPlaceInConfig(t *testing.T) {
	s := placedStore()
	a := withAttr(ent(t, KindCluster, "a"), "region", "EU-West-1")
	b := withAttr(ent(t, KindHost, "b"), "city", "singapore")
	c := withAttr(ent(t, KindHost, "c"), "region", "mars-1")
	snap := apply(t, s, ChangeSet{Upserts: []Entity{a, b, c}})
	for r, want := range map[EntityRef]Place{a.Ref: from(dublin, FromConfig), b.Ref: from(singapore, FromConfig), c.Ref: {}} {
		if got := placeOf(t, snap, r); got != want {
			t.Errorf("%s is at %+v; want %+v", r, got, want)
		}
	}
}

func TestTheFirstAttributeNamingAPlaceWins(t *testing.T) {
	s := NewStore(StoreOptions{Places: NewGazetteer([]string{"zone", "city"}, map[string]Place{"dublin": dublin, "singapore": singapore})})
	a := ent(t, KindHost, "a")
	a.Attrs = map[string]Value{"city": String("singapore"), "zone": String("dublin"), "region": String("nowhere")}
	if got := placeOf(t, apply(t, s, ChangeSet{Upserts: []Entity{a}}), a.Ref); got != from(dublin, FromConfig) {
		t.Errorf("placed at %+v; want dublin by zone", got)
	}
}

func TestAModulesPlaceWinsOverConfig(t *testing.T) {
	s := placedStore()
	a := withAttr(ent(t, KindHost, "a"), "region", "eu-west-1")
	a.Place = singapore
	if got := placeOf(t, apply(t, s, ChangeSet{Upserts: []Entity{a}}), a.Ref); got != from(singapore, FromModule) {
		t.Errorf("placed at %+v; want the module's", got)
	}
}

func TestAMemberTakesTheFirstPlacedOwnersPlace(t *testing.T) {
	s := placedStore()
	bare := ent(t, KindCluster, "bare")
	eu := withAttr(ent(t, KindCluster, "eu"), "region", "eu-west-1")
	sg := withAttr(ent(t, KindCluster, "sg"), "region", "singapore")
	h := ent(t, KindHost, "h")
	snap := apply(t, s, ChangeSet{
		Upserts: []Entity{bare, eu, sg, h},
		Edges:   []Edge{edge(h.Ref, bare.Ref, RelMemberOf), edge(h.Ref, sg.Ref, RelMemberOf), edge(h.Ref, eu.Ref, RelMemberOf)},
	})
	if got := placeOf(t, snap, h.Ref); got != from(singapore, FromMembership) {
		t.Errorf("the host is at %+v; want singapore, its first placed owner's", got)
	}
}

func TestMembershipIsOneStep(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	h := ent(t, KindHost, "h")
	p := ent(t, KindProcess, "p")
	snap := apply(t, s, ChangeSet{
		Upserts: []Entity{c, h, p},
		Edges:   []Edge{edge(h.Ref, c.Ref, RelMemberOf), edge(p.Ref, h.Ref, RelMemberOf)},
	})
	if got := placeOf(t, snap, p.Ref); got.Known {
		t.Errorf("a member of a member is placed at %+v", got)
	}
}

func TestAMemberFollowsItsOwnersPlace(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	h := ent(t, KindHost, "h")
	apply(t, s, ChangeSet{Upserts: []Entity{c, h}, Edges: []Edge{edge(h.Ref, c.Ref, RelMemberOf)}})

	c = withAttr(c, "region", "singapore")
	snap := apply(t, s, ChangeSet{Upserts: []Entity{c}})
	if got := placeOf(t, snap, h.Ref); got != from(singapore, FromMembership) || !slices.Contains(snap.Diff().Changed, h.Ref) {
		t.Errorf("after its cluster moved the host is at %+v, diff %+v", got, snap.Diff())
	}

	c.Attrs = nil
	snap = apply(t, s, ChangeSet{Upserts: []Entity{c}})
	if got := placeOf(t, snap, h.Ref); got.Known || !slices.Equal(snap.Diff().Changed, []EntityRef{c.Ref, h.Ref}) {
		t.Errorf("after its cluster lost its place the host is at %+v, diff %+v", got, snap.Diff())
	}
}

func TestAMemberLosesItsPlaceWithItsOwner(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	h := ent(t, KindHost, "h")
	m := edge(h.Ref, c.Ref, RelMemberOf)
	apply(t, s, ChangeSet{Upserts: []Entity{c, h}, Edges: []Edge{m}})
	snap := apply(t, s, ChangeSet{RemoveEdges: []EdgeKey{m.Key()}})
	if got := placeOf(t, snap, h.Ref); got.Known {
		t.Errorf("without its membership the host is at %+v", got)
	}
	apply(t, s, ChangeSet{Edges: []Edge{m}})
	snap = apply(t, s, ChangeSet{Removes: []EntityRef{c.Ref}})
	if got := placeOf(t, snap, h.Ref); got.Known {
		t.Errorf("without its cluster the host is at %+v", got)
	}
}

func TestAMemberIsPlacedWhenItsOwnerArrivesLater(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	h := ent(t, KindHost, "h")
	apply(t, s, ChangeSet{Upserts: []Entity{h}, Edges: []Edge{edge(h.Ref, c.Ref, RelMemberOf)}})
	snap := apply(t, s, ChangeSet{Upserts: []Entity{c}})
	if got := placeOf(t, snap, h.Ref); got != from(dublin, FromMembership) {
		t.Errorf("the host is at %+v once its cluster arrived", got)
	}
}

func TestAResentMemberKeepsItsPlaceUnchanged(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	h := ent(t, KindHost, "h")
	apply(t, s, ChangeSet{Upserts: []Entity{c, h}, Edges: []Edge{edge(h.Ref, c.Ref, RelMemberOf)}})
	snap := apply(t, s, ChangeSet{Upserts: []Entity{c, h}})
	if d := snap.Diff(); len(d.Changed) != 0 {
		t.Errorf("resending the same entities changed %v", d.Changed)
	}
	if got := placeOf(t, snap, h.Ref); got != from(dublin, FromMembership) {
		t.Errorf("the host is at %+v", got)
	}
}

func TestAResolvedPlaceSentBackIsNotTheModules(t *testing.T) {
	s := placedStore()
	c := withAttr(ent(t, KindCluster, "c"), "region", "eu-west-1")
	snap := apply(t, s, ChangeSet{Upserts: []Entity{c}})
	echo := *mustEntity(t, snap, c.Ref)
	echo.Attrs = nil
	if got := placeOf(t, apply(t, s, ChangeSet{Upserts: []Entity{echo}}), c.Ref); got.Known {
		t.Errorf("a config place sent back stuck as %+v", got)
	}
}

func TestAPlaceFromAnUnknownSourceIsInvalid(t *testing.T) {
	p := dublin
	p.From = FromMembership + 1
	if p.Validate() == nil {
		t.Error("a place from an unknown source is valid")
	}
}

func mustEntity(t *testing.T, snap *Snapshot, r EntityRef) *Entity {
	t.Helper()
	e, ok := snap.Entity(r)
	if !ok {
		t.Fatalf("%s is missing", r)
	}
	return e
}

func TestTheGazetteerFindsANameIgnoringCaseAndNamesAPlace(t *testing.T) {
	g := NewGazetteer(nil, map[string]Place{"eu-west-1": dublin, "Singapore": singapore, "SG": singapore})
	name, p, ok := g.Find("SINGAPORE")
	if !ok || name != "Singapore" || p != from(singapore, FromConfig) {
		t.Errorf("Find(SINGAPORE) = %q, %+v, %v; want Singapore at %+v", name, p, ok, singapore)
	}
	if _, _, ok := g.Find("mars-1"); ok {
		t.Error("found mars-1, which config does not name")
	}
	for p, want := range map[Place]string{from(singapore, FromMembership): "SG", dublin: "eu-west-1", At(0, 0): ""} {
		if got, _ := g.NameAt(p); got != want {
			t.Errorf("NameAt(%+v) = %q; want %q, the least name there", p, got, want)
		}
	}
	if got := g.Names([]string{"x"}); !slices.Equal(got, []string{"x", "SG", "Singapore", "eu-west-1"}) {
		t.Errorf("Names = %q; want each name as config writes it, in order, after x", got)
	}
	var none *Gazetteer
	if _, _, ok := none.Find("x"); ok {
		t.Error("no gazetteer found a name")
	}
	if _, ok := none.NameAt(dublin); ok {
		t.Error("no gazetteer named a place")
	}
}

func TestNewPlacesMoveWhatConfigPlaced(t *testing.T) {
	s := placedStore()
	eu := withAttr(ent(t, KindCluster, "eu"), "region", "eu-west-1")
	sg := withAttr(ent(t, KindCluster, "sg"), "region", "singapore")
	own := withAttr(ent(t, KindHost, "own"), "region", "eu-west-1")
	own.Place = dublin
	h := ent(t, KindHost, "h")
	before := apply(t, s, ChangeSet{Upserts: []Entity{eu, sg, own, h}, Edges: []Edge{edge(h.Ref, eu.Ref, RelMemberOf)}})
	snap := s.SetPlaces(NewGazetteer(nil, map[string]Place{"eu-west-1": singapore}))
	if snap.Version() == before.Version() {
		t.Fatal("new places made no new snapshot")
	}
	for r, want := range map[EntityRef]Place{
		eu.Ref: from(singapore, FromConfig), h.Ref: from(singapore, FromMembership),
		sg.Ref: {}, own.Ref: from(dublin, FromModule),
	} {
		if got := placeOf(t, snap, r); got != want {
			t.Errorf("%s is at %+v; want %+v", r, got, want)
		}
	}
	if got := placeOf(t, apply(t, s, ChangeSet{Upserts: []Entity{sg}}), sg.Ref); got.Known {
		t.Errorf("a resent entity is at %+v; want the new places to hold", got)
	}
}

func TestNoPlacesUnplaceWhatConfigPlaced(t *testing.T) {
	s := placedStore()
	eu := withAttr(ent(t, KindCluster, "eu"), "region", "eu-west-1")
	apply(t, s, ChangeSet{Upserts: []Entity{eu}})
	if got := placeOf(t, s.SetPlaces(nil), eu.Ref); got.Known {
		t.Errorf("placed at %+v; want nowhere", got)
	}
}

package model

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"testing/quick"
	"time"
)

func TestOnlyEntityAndEdgeChangesChangeTheShape(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	one := apply(t, s, ChangeSet{Upserts: []Entity{a, b}})
	events := apply(t, s, ChangeSet{Events: []Event{{ID: "e1", Entity: a.Ref, At: time.Unix(1, 0), Severity: SevWarn, Kind: "k", Source: "lh"}}})
	if !events.SameShape(one) || !one.SameShape(events) {
		t.Error("an events-only change changed the shape")
	}
	linked := apply(t, s, ChangeSet{Edges: []Edge{edge(a.Ref, b.Ref, RelDependsOn)}})
	if linked.SameShape(events) {
		t.Error("a new edge kept the shape")
	}
	a.Status.Level = StatusWarn
	if apply(t, s, ChangeSet{Upserts: []Entity{a}}).SameShape(linked) {
		t.Error("a changed entity kept the shape")
	}
}

func TestShapesFromDifferentStoresDiffer(t *testing.T) {
	x, y := NewStore(StoreOptions{}).Current(), NewStore(StoreOptions{}).Current()
	if x.SameShape(y) || x.SameShape(nil) {
		t.Error("unrelated snapshots share a shape")
	}
}

func TestOnlyAddingOrRemovingEntitiesAndEdgesChangesTheGraph(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	linked := apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{edge(a.Ref, b.Ref, RelDependsOn)}})
	a.Status.Level = StatusWarn
	heavier := edge(a.Ref, b.Ref, RelDependsOn)
	heavier.Weight = 2
	changed := apply(t, s, ChangeSet{Upserts: []Entity{a}, Edges: []Edge{heavier}})
	if !changed.SameGraph(linked) || changed.SameShape(linked) {
		t.Error("a changed entity and edge changed the graph, or kept the shape")
	}
	c := ent(t, KindHost, "c")
	grown := apply(t, s, ChangeSet{Upserts: []Entity{c}})
	if grown.SameGraph(changed) {
		t.Error("a new entity kept the graph")
	}
	cut := apply(t, s, ChangeSet{RemoveEdges: []EdgeKey{heavier.Key()}})
	if cut.SameGraph(grown) {
		t.Error("a removed edge kept the graph")
	}
	if cut.SameGraph(NewStore(StoreOptions{}).Current()) || cut.SameGraph(nil) {
		t.Error("unrelated snapshots share a graph")
	}
}

func TestAppendChangedListsWhatDiffersBetweenTwoSnapshots(t *testing.T) {
	s := NewStore(StoreOptions{})
	var all []Entity
	for i := range 2000 {
		all = append(all, ent(t, KindHost, fmt.Sprint(i)))
	}
	before := apply(t, s, ChangeSet{Upserts: all})
	if got := before.AppendChanged(nil, before); len(got) != 0 {
		t.Errorf("a snapshot differs from itself in %v", got)
	}
	warn := all[7]
	warn.Status.Level = StatusWarn
	apply(t, s, ChangeSet{Upserts: []Entity{warn}})
	renamed := all[1500]
	renamed.Name = "renamed"
	added := ent(t, KindHost, "new")
	after := apply(t, s, ChangeSet{Upserts: []Entity{renamed, added, all[3]}, Removes: []EntityRef{all[42].Ref}})
	got := slices.Sorted(slices.Values(after.AppendChanged(nil, before)))
	want := []EntityRef{warn.Ref, renamed.Ref, added.Ref, all[42].Ref}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("changed: got %v, want %v", got, want)
	}
	if n := len(after.AppendChanged(nil, nil)); n != after.Len() {
		t.Errorf("changed since nothing: %d entities; want all %d", n, after.Len())
	}
}

// TestAppendChangedMatchesComparingEveryEntity checks AppendChanged against a full comparison
// across random change sequences.
func TestAppendChangedMatchesComparingEveryEntity(t *testing.T) {
	check := func(seq changes) bool {
		s := NewStore(StoreOptions{})
		before := s.Current()
		for i, c := range seq {
			if i == len(seq)/2 {
				before = s.Current()
			}
			_, _ = s.Apply(&c)
		}
		after := s.Current()
		var want []EntityRef
		for _, e := range universe {
			x, inX := before.Entity(e.Ref)
			y, inY := after.Entity(e.Ref)
			if inX != inY || x != y {
				want = append(want, e.Ref)
			}
		}
		slices.Sort(want)
		return slices.Equal(slices.Sorted(slices.Values(after.AppendChanged(nil, before))), want)
	}
	if err := quick.Check(check, nil); err != nil {
		t.Error(err)
	}
}

// TestAppendChangedMatchesComparingEveryEntityInALargeWorld checks AppendChanged against a full
// comparison while random entities of thousands are changed, removed and added back.
func TestAppendChangedMatchesComparingEveryEntityInALargeWorld(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	s := NewStore(StoreOptions{})
	var all []Entity
	for i := range 3000 {
		all = append(all, ent(t, KindHost, fmt.Sprint(i)))
	}
	apply(t, s, ChangeSet{Upserts: all[:2000]})
	for round := range 20 {
		before := s.Current()
		var c ChangeSet
		for range r.IntN(40) {
			e := all[r.IntN(len(all))]
			switch r.IntN(3) {
			case 0:
				c.Removes = append(c.Removes, e.Ref)
			default:
				e.Name = fmt.Sprint(round)
				c.Upserts = append(c.Upserts, e)
			}
		}
		after := apply(t, s, c)
		var want []EntityRef
		for _, e := range all {
			x, inX := before.Entity(e.Ref)
			y, inY := after.Entity(e.Ref)
			if inX != inY || x != y {
				want = append(want, e.Ref)
			}
		}
		slices.Sort(want)
		if got := slices.Sorted(slices.Values(after.AppendChanged(nil, before))); !slices.Equal(got, want) {
			t.Fatalf("round %d: got %d changed, want %d", round, len(got), len(want))
		}
	}
}

func TestAppendChangedDoesNotAllocateIntoRoom(t *testing.T) {
	s := NewStore(StoreOptions{})
	var all []Entity
	for i := range 2000 {
		all = append(all, ent(t, KindHost, fmt.Sprint(i)))
	}
	before := apply(t, s, ChangeSet{Upserts: all})
	e := all[9]
	e.Name = "changed"
	after := apply(t, s, ChangeSet{Upserts: []Entity{e}})
	buf := make([]EntityRef, 0, 4)
	if n := testing.AllocsPerRun(10, func() { buf = after.AppendChanged(buf[:0], before) }); n != 0 {
		t.Errorf("%v allocations", n)
	}
}

package model

import (
	"slices"
	"strconv"
	"testing"
)

func TestBatchReplaceDropsWhatTheSnapshotOmits(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := host("lh", "a", "a", nil), host("lh", "b", "b", nil)
	x := host("other", "x", "x", nil)
	ab, xa := edge(a.Ref, b.Ref, RelDependsOn), edge(x.Ref, a.Ref, RelDependsOn) // both owned by lh
	apply(t, s, ChangeSet{Upserts: []Entity{a, b, x}, Edges: []Edge{ab, xa}})

	var batch Batch
	c := host("lh", "c", "c", nil)
	batch.Add(&ChangeSet{Upserts: []Entity{c}})
	batch.Replace("lh", s.Current(), &ChangeSet{Upserts: []Entity{a}})
	cs := batch.ChangeSet()
	snap := apply(t, s, cs)

	if got := refs(snap.BySource("lh")); !slices.Equal(got, []EntityRef{a.Ref}) {
		t.Errorf("lh owns %v, want only %s", got, a.Ref)
	}
	if _, ok := snap.Entity(x.Ref); !ok {
		t.Error("another module's entity was removed")
	}
	if n := snap.EdgeLen(); n != 0 {
		t.Errorf("%d edges survive; the snapshot declared none", n)
	}
}

func TestBatchReplaceKeepsWhatTheSnapshotDeclares(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := host("lh", "a", "a", nil), host("lh", "b", "b", nil)
	ab := edge(a.Ref, b.Ref, RelDependsOn)
	apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{ab}})

	var batch Batch
	batch.Replace("lh", s.Current(), &ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{ab}})
	snap := apply(t, s, batch.ChangeSet())
	d := snap.Diff()
	if snap.Len() != 2 || snap.EdgeLen() != 1 || len(d.Removed)+len(d.Changed)+len(d.EdgesRemoved) != 0 {
		t.Errorf("len %d, edges %d, diff %+v; want an unchanged world", snap.Len(), snap.EdgeLen(), snap.Diff())
	}
}

func TestBatchChangeSetReusesStorage(t *testing.T) {
	var b Batch
	a, c := ent(t, KindHost, "a"), ent(t, KindHost, "c")
	cs := ChangeSet{Upserts: []Entity{a, c}, Edges: []Edge{edge(a.Ref, c.Ref, RelTalksTo)}, Removes: []EntityRef{ref(t, KindDisk, "d")}}
	round := func() {
		b.Add(&cs)
		out := b.ChangeSet()
		if len(out.Upserts) != 2 || len(out.Edges) != 1 || len(out.Removes) != 1 {
			t.Fatalf("change set = %+v", out)
		}
		b.Reset()
	}
	round()
	if allocs := testing.AllocsPerRun(20, round); allocs != 0 {
		t.Errorf("a warm batch allocates %v times per round, want 0", allocs)
	}
}

// A snapshot's batch is far larger than any delta after it; keeping its storage for reuse
// would hold the whole world a second time for as long as the app runs.
func TestResetReleasesASnapshotsStorage(t *testing.T) {
	var b Batch
	c := &ChangeSet{}
	for i := range keepLimit + 1 {
		r, _ := NewEntityRef("m", KindHost, strconv.Itoa(i))
		c.Upserts = append(c.Upserts, Entity{Ref: r, Kind: KindHost, Source: "m"})
	}
	b.Add(c)
	b.ChangeSet()
	b.Reset()
	if b.ents != nil || b.out.Upserts != nil {
		t.Error("Reset kept a snapshot-sized batch's storage")
	}
	b.Add(&ChangeSet{Upserts: c.Upserts[:1]})
	b.ChangeSet()
	b.Reset()
	if b.ents == nil {
		t.Error("Reset dropped a small batch's maps, which deltas reuse")
	}
}

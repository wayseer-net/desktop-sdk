package model

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"
)

func ref(t testing.TB, kind Kind, native string) EntityRef {
	t.Helper()
	r, err := NewEntityRef("lh", kind, native)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ent(t testing.TB, kind Kind, native string) Entity {
	return Entity{Ref: ref(t, kind, native), Kind: kind, Name: native, Source: "lh"}
}

func edge(from, to EntityRef, rel Relation) Edge {
	return Edge{From: from, To: to, Rel: rel, Source: "lh"}
}

func apply(t testing.TB, s *Store, c ChangeSet) *Snapshot {
	t.Helper()
	snap, err := s.Apply(&c)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func sorted[T any](seq func(func(T) bool), cmp func(a, b T) int) []T {
	var out []T
	for v := range seq {
		out = append(out, v)
	}
	slices.SortFunc(out, cmp)
	return out
}

func refs(seq func(func(EntityRef) bool)) []EntityRef {
	return sorted(seq, func(a, b EntityRef) int { return compareStr(a, b) })
}

func TestApplyUpsertIndexesAndReplaces(t *testing.T) {
	s := NewStore(StoreOptions{})
	host, disk := ent(t, KindHost, "me"), ent(t, KindDisk, "nvme0")
	host.Attrs = map[string]Value{"cores": Number(16)}
	snap := apply(t, s, ChangeSet{Upserts: []Entity{host, disk}})
	if snap.Version() != 1 || snap.Len() != 2 || s.Current() != snap {
		t.Fatalf("version %d len %d", snap.Version(), snap.Len())
	}
	if got := refs(snap.ByKind(KindDisk)); !slices.Equal(got, []EntityRef{disk.Ref}) {
		t.Errorf("ByKind(disk) = %v", got)
	}
	if got := refs(snap.BySource("lh")); len(got) != 2 {
		t.Errorf("BySource = %v", got)
	}
	host.Name = "renamed"
	snap = apply(t, s, ChangeSet{Upserts: []Entity{host}})
	if e, _ := snap.Entity(host.Ref); e.Name != "renamed" || snap.Len() != 2 {
		t.Errorf("upsert did not replace: %+v", e)
	}
}

func TestRemoveEntityRemovesItsEdges(t *testing.T) {
	s := NewStore(StoreOptions{})
	host, p1, p2 := ent(t, KindHost, "me"), ent(t, KindProcess, "1"), ent(t, KindProcess, "2")
	snap := apply(t, s, ChangeSet{
		Upserts: []Entity{host, p1, p2},
		Edges:   []Edge{edge(p1.Ref, host.Ref, RelRunsOn), edge(p2.Ref, host.Ref, RelRunsOn), edge(p1.Ref, p2.Ref, RelParentOf)},
	})
	if snap.EdgeLen() != 3 || len(slices.Collect(snap.In(host.Ref))) != 2 {
		t.Fatalf("edges %d, in(host) %d", snap.EdgeLen(), len(slices.Collect(snap.In(host.Ref))))
	}
	snap = apply(t, s, ChangeSet{Removes: []EntityRef{p1.Ref}})
	if snap.EdgeLen() != 1 {
		t.Errorf("edges after removing p1 = %d, want 1", snap.EdgeLen())
	}
	in := slices.Collect(snap.In(host.Ref))
	if len(in) != 1 || in[0].From != p2.Ref {
		t.Errorf("in(host) = %v, want only p2", in)
	}
	if len(slices.Collect(snap.Out(p2.Ref))) != 1 || len(slices.Collect(snap.In(p2.Ref))) != 0 {
		t.Error("p2's adjacency should lose the parent edge")
	}
	if len(slices.Collect(snap.ByKind(KindProcess))) != 1 {
		t.Error("kind index still lists p1")
	}
}

func TestRemoveEdge(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	e := edge(a.Ref, b.Ref, RelTalksTo)
	apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{e}})
	snap := apply(t, s, ChangeSet{RemoveEdges: []EdgeKey{e.Key()}})
	if snap.EdgeLen() != 0 || len(slices.Collect(snap.Out(a.Ref))) != 0 {
		t.Error("edge not removed")
	}
	if _, ok := snap.Edge(e.Key()); ok {
		t.Error("Edge still finds the removed edge")
	}
}

func TestDanglingEdgeWaitsForEntity(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	e := edge(a.Ref, b.Ref, RelDependsOn)
	snap := apply(t, s, ChangeSet{Upserts: []Entity{a}, Edges: []Edge{e}})
	if snap.EdgeLen() != 0 {
		t.Fatal("an edge to a missing entity must not be visible")
	}
	snap = apply(t, s, ChangeSet{Upserts: []Entity{b}})
	if _, ok := snap.Edge(e.Key()); !ok || snap.EdgeLen() != 1 {
		t.Fatal("the edge should appear once its entity arrives")
	}
	if d := snap.Diff(); !slices.Equal(d.EdgesAdded, []EdgeKey{e.Key()}) {
		t.Errorf("diff edges added = %v", d.EdgesAdded)
	}
}

func TestDanglingEdgeDiesWithEndpoint(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	apply(t, s, ChangeSet{Upserts: []Entity{a}, Edges: []Edge{edge(a.Ref, b.Ref, RelDependsOn)}})
	apply(t, s, ChangeSet{Removes: []EntityRef{a.Ref}})
	snap := apply(t, s, ChangeSet{Upserts: []Entity{a, b}})
	if snap.EdgeLen() != 0 {
		t.Error("removing an endpoint must discard its dangling edges")
	}
}

func TestSnapshotIsolation(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	old := apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{edge(a.Ref, b.Ref, RelTalksTo)}})
	for i := range 50 {
		c := ent(t, KindDisk, string(rune('a'+i%26))+"x")
		apply(t, s, ChangeSet{Upserts: []Entity{c}, Edges: []Edge{edge(c.Ref, a.Ref, RelMemberOf)}, Removes: []EntityRef{b.Ref}})
	}
	if old.Len() != 2 || old.EdgeLen() != 1 || old.Version() != 1 {
		t.Fatalf("held snapshot changed: len %d edges %d", old.Len(), old.EdgeLen())
	}
	if _, ok := old.Entity(b.Ref); !ok || len(slices.Collect(old.In(a.Ref))) != 0 || len(slices.Collect(old.Out(a.Ref))) != 1 {
		t.Error("held snapshot's entities or adjacency changed")
	}
	if len(slices.Collect(old.ByKind(KindDisk))) != 0 {
		t.Error("held snapshot's kind index changed")
	}
}

// TestApplyTakesOwnership checks the store keeps a change set's maps and slices instead of
// copying them; callers hand them over and must not modify them afterwards.
func TestApplyTakesOwnership(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindHost, "a"), ent(t, KindHost, "b")
	a.Attrs, a.Tags = map[string]Value{"cpu": Number(1)}, []string{"web"}
	e := edge(a.Ref, b.Ref, RelTalksTo)
	e.Attrs = map[string]Value{"port": Number(443)}
	snap := apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{e}})
	got, _ := snap.Entity(a.Ref)
	if reflect.ValueOf(got.Attrs).UnsafePointer() != reflect.ValueOf(a.Attrs).UnsafePointer() || &got.Tags[0] != &a.Tags[0] {
		t.Error("entity attributes or tags were copied")
	}
	gotEdge, _ := snap.Edge(e.Key())
	if reflect.ValueOf(gotEdge.Attrs).UnsafePointer() != reflect.ValueOf(e.Attrs).UnsafePointer() {
		t.Error("edge attributes were copied")
	}
}

func TestDiffIsExact(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b, c := ent(t, KindHost, "a"), ent(t, KindHost, "b"), ent(t, KindHost, "c")
	apply(t, s, ChangeSet{Upserts: []Entity{a, b}})
	a2 := a
	a2.Status = Status{Level: StatusWarn}
	d := apply(t, s, ChangeSet{Upserts: []Entity{a2, b, c}, Removes: []EntityRef{}}).Diff()
	if !slices.Equal(d.Added, []EntityRef{c.Ref}) || !slices.Equal(d.Changed, []EntityRef{a.Ref}) || len(d.Removed) != 0 {
		t.Errorf("diff = %+v; an identical upsert of b is not a change", d)
	}
	d = apply(t, s, ChangeSet{Removes: []EntityRef{b.Ref, "lh/host/zzz"}, Upserts: []Entity{c}}).Diff()
	if !slices.Equal(d.Removed, []EntityRef{b.Ref}) || len(d.Changed) != 0 {
		t.Errorf("diff = %+v", d)
	}
	d = apply(t, s, ChangeSet{Removes: []EntityRef{c.Ref}, Upserts: []Entity{c}}).Diff()
	if !slices.Equal(d.Changed, []EntityRef{c.Ref}) || len(d.Added)+len(d.Removed) != 0 {
		t.Errorf("remove then re-add should be a change: %+v", d)
	}
}

func TestInvalidItemsRejectedOthersApplied(t *testing.T) {
	s := NewStore(StoreOptions{})
	good := ent(t, KindHost, "a")
	bad := Entity{Ref: "nope", Kind: KindHost, Source: "lh"}
	snap, err := s.Apply(&ChangeSet{Upserts: []Entity{bad, good}, Edges: []Edge{edge(good.Ref, good.Ref, RelOwns)}})
	if err == nil {
		t.Error("invalid entity and self edge should be reported")
	}
	if snap.Len() != 1 || snap.EdgeLen() != 0 {
		t.Errorf("len %d edges %d; valid items should still apply", snap.Len(), snap.EdgeLen())
	}
}

func TestEventLogIsBoundedAndIsolated(t *testing.T) {
	s := NewStore(StoreOptions{EventCap: 600})
	var held *Snapshot
	for i := range 2000 {
		snap := apply(t, s, ChangeSet{Events: []Event{{ID: string(rune(i)), At: time.Unix(int64(i), 0), Message: "m"}}})
		if i == 99 {
			held = snap
		}
	}
	evs := slices.Collect(s.Current().Events())
	if len(evs) < 600 || len(evs) > 600+eventChunk {
		t.Errorf("kept %d events, want about 600", len(evs))
	}
	if last := evs[len(evs)-1]; last.At.Unix() != 1999 {
		t.Errorf("newest event at %d", last.At.Unix())
	}
	if got := slices.Collect(held.Events()); len(got) != 100 || got[99].At.Unix() != 99 {
		t.Errorf("held snapshot sees %d events", len(got))
	}
	if s.Current().Diff().Events != 1 {
		t.Error("diff should count new events")
	}
}

func TestEventsAreNumberedInTheOrderAppended(t *testing.T) {
	s := NewStore(StoreOptions{EventCap: 600})
	for i := range 2000 {
		apply(t, s, ChangeSet{Events: []Event{{ID: strconv.Itoa(i), At: time.Unix(int64(i), 0)}}})
	}
	snap := s.Current()
	first, end := snap.EventRange()
	if end != 2000 || first == 0 || first%eventChunk != 0 {
		t.Fatalf("range %d to %d; want whole chunks forgotten, up to 2000", first, end)
	}
	for _, seq := range []uint64{first, 1500, end - 1} {
		if e, ok := snap.EventAt(seq); !ok || e.ID != strconv.FormatUint(seq, 10) {
			t.Errorf("event %d is %v %v", seq, e, ok)
		}
	}
	for _, seq := range []uint64{first - 1, end} {
		if _, ok := snap.EventAt(seq); ok {
			t.Errorf("event %d is not retained, but was found", seq)
		}
	}
	if n := testing.AllocsPerRun(10, func() { snap.EventAt(1500) }); n != 0 {
		t.Errorf("EventAt allocates %v times", n)
	}
}

func TestOldSnapshotsAreCollected(t *testing.T) {
	s := NewStore(StoreOptions{})
	rename := func(round int) {
		var c ChangeSet
		for i := range 1000 {
			e := ent(t, KindHost, fmt.Sprint(i))
			e.Name = fmt.Sprint(round) // every entity changes, so each version differs
			c.Upserts = append(c.Upserts, e)
		}
		apply(t, s, c)
	}
	rename(0)
	before := liveHeap()
	for round := 1; round <= 100; round++ {
		rename(round)
	}
	if grew := liveHeap() - before; grew > 4<<20 {
		t.Errorf("live heap grew %d KiB over 100 versions of a 1000-entity world; old snapshots leak", grew>>10)
	}
	runtime.KeepAlive(s) // otherwise the whole store is collected before measuring
}

func liveHeap() int64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc)
}

func TestARateAloneChangesAnEdge(t *testing.T) {
	s := NewStore(StoreOptions{})
	a, b := ent(t, KindService, "a"), ent(t, KindService, "b")
	e := edge(a.Ref, b.Ref, RelTalksTo)
	e.Traffic = Traffic{Rate: 10, Unit: TrafficRequests}
	apply(t, s, ChangeSet{Upserts: []Entity{a, b}, Edges: []Edge{e}})
	if snap := apply(t, s, ChangeSet{Edges: []Edge{e}}); len(snap.Diff().EdgesChanged) != 0 {
		t.Errorf("the same rate changed the edge: %+v", snap.Diff())
	}
	e.Traffic.Rate = 11
	snap := apply(t, s, ChangeSet{Edges: []Edge{e}})
	if !slices.Equal(snap.Diff().EdgesChanged, []EdgeKey{e.Key()}) {
		t.Errorf("diff %+v; want the edge changed", snap.Diff())
	}
	if got, _ := snap.Edge(e.Key()); got.Traffic != e.Traffic {
		t.Errorf("stored %+v", got.Traffic)
	}
}

func TestAPlaceAloneChangesAnEntity(t *testing.T) {
	s := NewStore(StoreOptions{})
	a := ent(t, KindHost, "a")
	apply(t, s, ChangeSet{Upserts: []Entity{a}})
	a.Place = At(1.35, 103.82)
	snap := apply(t, s, ChangeSet{Upserts: []Entity{a}})
	if !slices.Equal(snap.Diff().Changed, []EntityRef{a.Ref}) {
		t.Errorf("diff %+v; want the entity changed", snap.Diff())
	}
	if got, _ := snap.Entity(a.Ref); got.Place != a.Place {
		t.Errorf("stored %+v", got.Place)
	}
	if snap := apply(t, s, ChangeSet{Upserts: []Entity{a}}); len(snap.Diff().Changed) != 0 {
		t.Errorf("the same place changed the entity: %+v", snap.Diff())
	}
}

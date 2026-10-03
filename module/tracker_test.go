package module

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"wayseer.dev/sdk/model"
)

func host(native, name string) model.Entity {
	r, _ := model.NewEntityRef("t", model.KindHost, native)
	return model.Entity{Ref: r, Kind: model.KindHost, Name: name, Source: "t"}
}

func TestTrackerSendsOnlyChanges(t *testing.T) {
	a, b := host("a", "A"), host("b", "B")
	link := model.Edge{From: a.Ref, To: b.Ref, Rel: model.RelDependsOn, Weight: 1, Source: "t"}
	now := time.Unix(100, 0)
	var tr Tracker
	cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a, b.Ref: b}, map[model.EdgeKey]model.Edge{link.Key(): link}, now)
	if len(cs.Upserts) != 2 || len(cs.Edges) != 1 || !cs.Upserts[0].Seen.Equal(now) {
		t.Fatalf("first = %+v", cs)
	}
	b.Name = "B2"
	cs = tr.Changes(map[model.EntityRef]model.Entity{b.Ref: b}, nil, now)
	if !slices.Equal(cs.Removes, []model.EntityRef{a.Ref}) || len(cs.Upserts) != 1 || cs.Upserts[0].Name != "B2" {
		t.Errorf("second = %+v; want a removed and b updated", cs)
	}
	if !slices.Equal(cs.RemoveEdges, []model.EdgeKey{link.Key()}) {
		t.Errorf("edge removals = %v", cs.RemoveEdges)
	}
	if cs = tr.Changes(map[model.EntityRef]model.Entity{b.Ref: b}, nil, now); !cs.Empty() {
		t.Errorf("unchanged world sent %+v", cs)
	}
	tr.Reset()
	if cs = tr.Changes(map[model.EntityRef]model.Entity{b.Ref: b}, nil, now); len(cs.Upserts) != 1 {
		t.Errorf("after Reset = %+v; want everything resent", cs)
	}
}

func TestTrackerNoticesAttrAndTagChanges(t *testing.T) {
	a := host("a", "A")
	a.Attrs = map[string]model.Value{"n": model.Number(1)}
	var tr Tracker
	tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now())
	a.Attrs = map[string]model.Value{"n": model.Number(2)}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); len(cs.Upserts) != 1 {
		t.Error("attr change not sent")
	}
	a.Tags = []string{"x"}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); len(cs.Upserts) != 1 {
		t.Error("tag change not sent")
	}
}

func TestTrackerNoticesStatusAndValueTypeChanges(t *testing.T) {
	a := host("a", "A")
	a.Attrs = map[string]model.Value{"n": model.Number(16)}
	var tr Tracker
	send := func() int {
		return len(tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()).Upserts)
	}
	send()
	a.Attrs = map[string]model.Value{"n": model.String("16")}
	if send() != 1 {
		t.Error("a number becoming the same text was not sent")
	}
	a.Attrs = map[string]model.Value{"n": model.Number(16).In(model.UnitBytes)}
	send()
	a.Attrs = map[string]model.Value{"n": model.Number(16).In(model.UnitSeconds)}
	if send() != 1 {
		t.Error("a number changing its unit was not sent")
	}
	a.Status = model.Status{Level: model.StatusWarn, Reason: "hot"}
	if send() != 1 {
		t.Error("status change not sent")
	}
	a.Status.Reason = "hotter"
	if send() != 1 {
		t.Error("reason change not sent")
	}
	a.Tags, a.Name = []string{"ab"}, "A"
	send()
	a.Tags, a.Name = []string{"a", "b"}, "A"
	if send() != 1 {
		t.Error("tags split differently were taken as the same")
	}
}

func TestTrackerIgnoresSeenAndAttrOrder(t *testing.T) {
	a := host("a", "A")
	a.Attrs = map[string]model.Value{"x": model.Number(1), "y": model.String("two"), "z": model.Bool(true)}
	var tr Tracker
	tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now())
	a.Seen = time.Now().Add(time.Hour)
	a.Attrs = map[string]model.Value{"z": model.Bool(true), "y": model.String("two"), "x": model.Number(1)}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); !cs.Empty() {
		t.Errorf("an equal entity was resent: %+v", cs)
	}
}

func TestTrackerAttrsDoNotCancelOut(t *testing.T) {
	a := host("a", "A")
	a.Attrs = map[string]model.Value{"x": model.Number(1), "y": model.Number(1)}
	var tr Tracker
	tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now())
	a.Attrs = map[string]model.Value{"x": model.Number(2), "y": model.Number(2)}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); len(cs.Upserts) != 1 {
		t.Error("two attributes changing together were missed")
	}
}

func TestTrackerKeepsListItemsApart(t *testing.T) {
	a := host("a", "A")
	a.Attrs = map[string]model.Value{"l": model.List(model.String("x, y"))}
	var tr Tracker
	tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now())
	a.Attrs = map[string]model.Value{"l": model.List(model.String("x"), model.String("y"))}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); len(cs.Upserts) != 1 {
		t.Error("a list split differently was taken as the same")
	}
}

func TestTrackerChangesOfMatchesChanges(t *testing.T) {
	a, b := host("a", "A"), host("b", "B")
	link := model.Edge{From: a.Ref, To: b.Ref, Rel: model.RelDependsOn, Weight: 1, Source: "t"}
	var byValue, byPointer Tracker
	now := time.Unix(100, 0)
	want := byValue.Changes(map[model.EntityRef]model.Entity{a.Ref: a, b.Ref: b}, map[model.EdgeKey]model.Edge{link.Key(): link}, now)
	got := byPointer.ChangesOf(map[model.EntityRef]*model.Entity{a.Ref: &a, b.Ref: &b}, map[model.EdgeKey]*model.Edge{link.Key(): &link}, now)
	sameUpsert := func(x, y model.Entity) bool { return x.Ref == y.Ref && x.Name == y.Name && x.Seen.Equal(y.Seen) }
	sameEdge := func(x, y model.Edge) bool { return x.Key() == y.Key() }
	if !slices.EqualFunc(got.Upserts, want.Upserts, sameUpsert) || !slices.EqualFunc(got.Edges, want.Edges, sameEdge) {
		t.Errorf("by pointer %+v, by value %+v", got, want)
	}
	if cs := byPointer.ChangesOf(map[model.EntityRef]*model.Entity{a.Ref: &a}, nil, now); len(cs.Removes) != 1 || len(cs.RemoveEdges) != 1 {
		t.Errorf("removals by pointer = %+v", cs)
	}
}

// BenchmarkTrackerUnchanged10k times a poll that finds nothing changed, as most polls do.
func BenchmarkTrackerUnchanged10k(b *testing.B) {
	ents := map[model.EntityRef]model.Entity{}
	edges := map[model.EdgeKey]model.Edge{}
	var prev model.EntityRef
	for i := range 10000 {
		e := host(strconv.Itoa(i), "host "+strconv.Itoa(i))
		e.Attrs = map[string]model.Value{"cores": model.Number(float64(i % 64)), "os": model.String("debian")}
		e.Tags = []string{"web", "eu"}
		ents[e.Ref] = e
		if i > 0 {
			ed := model.Edge{From: e.Ref, To: prev, Rel: model.RelDependsOn, Weight: 1, Source: "t"}
			edges[ed.Key()] = ed
		}
		prev = e.Ref
	}
	var tr Tracker
	tr.Changes(ents, edges, time.Now())
	b.ReportAllocs()
	for b.Loop() {
		if cs := tr.Changes(ents, edges, time.Now()); !cs.Empty() {
			b.Fatal("an unchanged world sent changes")
		}
	}
}

func TestTrackerSendsInAStableOrder(t *testing.T) {
	ents := map[model.EntityRef]model.Entity{}
	edges := map[model.EdgeKey]model.Edge{}
	var prev model.Entity
	for i := range 50 {
		e := host(strconv.Itoa(i), "h")
		ents[e.Ref] = e
		if i > 0 {
			ed := model.Edge{From: e.Ref, To: prev.Ref, Rel: model.RelDependsOn, Weight: 1, Source: "t"}
			edges[ed.Key()] = ed
		}
		prev = e
	}
	var tr Tracker
	cs := tr.Changes(ents, edges, time.Now())
	byRef := func(a, b model.Entity) int { return strings.Compare(string(a.Ref), string(b.Ref)) }
	if !slices.IsSortedFunc(cs.Upserts, byRef) || !slices.IsSortedFunc(cs.Edges, func(a, b model.Edge) int { return CompareEdgeKeys(a.Key(), b.Key()) }) {
		t.Error("upserts or edges out of order")
	}
	cs = tr.Changes(nil, nil, time.Now())
	if !slices.IsSorted(cs.Removes) || !slices.IsSortedFunc(cs.RemoveEdges, CompareEdgeKeys) || len(cs.Removes) != 50 {
		t.Errorf("%d removals, or out of order", len(cs.Removes))
	}
}

func TestTrackerResendsAnEdgeWhoseTrafficChanged(t *testing.T) {
	a, b := host("a", "A"), host("b", "B")
	ents := map[model.EntityRef]model.Entity{a.Ref: a, b.Ref: b}
	link := model.Edge{From: a.Ref, To: b.Ref, Rel: model.RelTalksTo, Source: "t", Traffic: model.Traffic{Rate: 5, Unit: model.TrafficRequests}}
	var tr Tracker
	tr.Changes(ents, map[model.EdgeKey]model.Edge{link.Key(): link}, time.Now())
	if cs := tr.Changes(ents, map[model.EdgeKey]model.Edge{link.Key(): link}, time.Now()); !cs.Empty() {
		t.Errorf("unchanged traffic sent %+v", cs)
	}
	link.Traffic.Rate = 7
	if cs := tr.Changes(ents, map[model.EdgeKey]model.Edge{link.Key(): link}, time.Now()); len(cs.Edges) != 1 || cs.Edges[0].Traffic.Rate != 7 {
		t.Errorf("changed traffic sent %+v", cs)
	}
}

func TestTrackerResendsAnEntityWhosePlaceChanged(t *testing.T) {
	a := host("a", "A")
	var tr Tracker
	tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now())
	a.Place = model.At(52.37, 4.9)
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); len(cs.Upserts) != 1 || cs.Upserts[0].Place != a.Place {
		t.Errorf("a new place sent %+v", cs)
	}
	if cs := tr.Changes(map[model.EntityRef]model.Entity{a.Ref: a}, nil, time.Now()); !cs.Empty() {
		t.Errorf("an unchanged place sent %+v", cs)
	}
}

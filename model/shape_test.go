package model

import (
	"testing"
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

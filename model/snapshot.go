package model

import (
	"iter"
	"slices"
)

// Snapshot is an immutable view of the world at one version; it is safe to read from any
// goroutine. Returned entities and edges are shared and must not be modified.
type Snapshot struct {
	version  uint64
	diff     Diff
	entities pmap[EntityRef, *Entity]
	edges    pmap[EdgeKey, *Edge]
	adj      pmap[EntityRef, *adjacency]
	byKind   pmap[Kind, refSet]
	bySource pmap[ModuleID, refSet]
	grouped  refSet // entities with a same_as edge
	events   eventView
	shape    *shape // shared by snapshots whose entities and edges are the same
}

// shape marks one set of entities and edges; it is not empty, so each is a distinct pointer.
type shape struct{ since uint64 }

// SameShape reports whether s and o have the same entities and edges, as when only events
// arrived between them. Snapshots from different stores never do.
func (s *Snapshot) SameShape(o *Snapshot) bool { return s != nil && o != nil && s.shape == o.shape }

type refSet = pmap[EntityRef, struct{}]

// adjacency lists an entity's edges as of its snapshot; copied on write, like pmap nodes.
type adjacency struct {
	owner   *edit
	out, in []*Edge
}

// Diff lists what changed since the previous snapshot, each list sorted.
type Diff struct {
	Added, Changed, Removed                []EntityRef
	EdgesAdded, EdgesChanged, EdgesRemoved []EdgeKey
	Events                                 int // events appended
}

// Version counts transactions; the empty initial snapshot is version 0.
func (s *Snapshot) Version() uint64 { return s.version }

// Diff describes the transaction that produced s.
func (s *Snapshot) Diff() *Diff { return &s.diff }

// Len is the number of entities.
func (s *Snapshot) Len() int { return s.entities.len() }

// EdgeLen is the number of edges whose endpoints both exist.
func (s *Snapshot) EdgeLen() int { return s.edges.len() }

// Entity looks up an entity by ref.
func (s *Snapshot) Entity(r EntityRef) (*Entity, bool) { return s.entities.get(r) }

// Edge looks up an edge by key.
func (s *Snapshot) Edge(k EdgeKey) (*Edge, bool) { return s.edges.get(k) }

// Entities yields every entity in unspecified order.
func (s *Snapshot) Entities() iter.Seq[*Entity] { return values(s.entities.all()) }

// Edges yields every edge in unspecified order.
func (s *Snapshot) Edges() iter.Seq[*Edge] { return values(s.edges.all()) }

// ByKind yields the refs of every entity of kind k.
func (s *Snapshot) ByKind(k Kind) iter.Seq[EntityRef] {
	set, _ := s.byKind.get(k)
	return keys(set.all())
}

// BySource yields the refs of every entity from module m.
func (s *Snapshot) BySource(m ModuleID) iter.Seq[EntityRef] {
	set, _ := s.bySource.get(m)
	return keys(set.all())
}

// SourceLen returns how many entities module m has.
func (s *Snapshot) SourceLen(m ModuleID) int {
	set, _ := s.bySource.get(m)
	return set.len()
}

// Grouped yields the refs of every entity with a same_as edge.
func (s *Snapshot) Grouped() iter.Seq[EntityRef] { return keys(s.grouped.all()) }

// Out yields the edges leaving r.
func (s *Snapshot) Out(r EntityRef) iter.Seq[*Edge] {
	a, _ := s.adj.get(r)
	return edgesOf(a, false)
}

// In yields the edges arriving at r.
func (s *Snapshot) In(r EntityRef) iter.Seq[*Edge] {
	a, _ := s.adj.get(r)
	return edgesOf(a, true)
}

func edgesOf(a *adjacency, in bool) iter.Seq[*Edge] {
	return func(yield func(*Edge) bool) {
		if a == nil {
			return
		}
		list := a.out
		if in {
			list = a.in
		}
		for _, e := range list {
			if !yield(e) {
				return
			}
		}
	}
}

// Events yields the retained events, oldest first.
func (s *Snapshot) Events() iter.Seq[*Event] { return s.events.all() }

// EventRange numbers the retained events in the order appended: the oldest, and one past the
// newest. Numbers are never reused, so a reader can take only the events new since it last looked.
func (s *Snapshot) EventRange() (first, end uint64) { return s.events.bounds() }

// EventAt is the event numbered seq, if still retained.
func (s *Snapshot) EventAt(seq uint64) (*Event, bool) { return s.events.at(seq) }

func values[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}

func keys[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

// SameAs is r's group: r and every entity linked to it through same_as edges, sorted.
func (s *Snapshot) SameAs(r EntityRef) []EntityRef {
	group := []EntityRef{r}
	for i := 0; i < len(group); i++ {
		for _, e := range s.sameAsOf(group[i]) {
			if !slices.Contains(group, e) {
				group = append(group, e)
			}
		}
	}
	slices.Sort(group)
	return group
}

// sameAsOf are the entities at the other end of r's same_as edges.
func (s *Snapshot) sameAsOf(r EntityRef) []EntityRef {
	var out []EntityRef
	for e := range s.Out(r) {
		if e.Rel == RelSameAs {
			out = append(out, e.To)
		}
	}
	for e := range s.In(r) {
		if e.Rel == RelSameAs {
			out = append(out, e.From)
		}
	}
	return out
}

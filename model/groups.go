package model

import (
	"cmp"
	"slices"
)

// Groups are a snapshot's same_as groups, each shown as one entity: its face, the member with
// the most other edges, though a process gives way to what it runs. A nil Groups leaves every
// entity its own.
type Groups struct {
	world   *Snapshot
	face    map[EntityRef]EntityRef   // every grouped entity's face
	members map[EntityRef][]EntityRef // by face: the face, then the others sorted
}

// GroupsOf finds s's groups, visiting only the entities that have same_as edges.
func GroupsOf(s *Snapshot) *Groups {
	g := &Groups{world: s, face: map[EntityRef]EntityRef{}, members: map[EntityRef][]EntityRef{}}
	for r := range s.Grouped() {
		if _, done := g.face[r]; done {
			continue
		}
		group := s.SameAs(r)
		face := slices.MaxFunc(group, func(a, b EntityRef) int {
			return cmp.Or(cmp.Compare(notProcess(s, a), notProcess(s, b)), cmp.Compare(s.degree(a), s.degree(b)), cmp.Compare(b, a))
		})
		for _, m := range group {
			g.face[m] = face
		}
		i := slices.Index(group, face)
		g.members[face] = append([]EntityRef{face}, slices.Delete(group, i, i+1)...)
	}
	return g
}

// notProcess is 1 unless r is a process, which a group shows by what it runs.
func notProcess(s *Snapshot, r EntityRef) int {
	if e, ok := s.Entity(r); ok && e.Kind == KindProcess {
		return 0
	}
	return 1
}

// degree counts r's edges other than same_as.
func (s *Snapshot) degree(r EntityRef) int {
	n := 0
	for e := range s.Out(r) {
		if e.Rel != RelSameAs {
			n++
		}
	}
	for e := range s.In(r) {
		if e.Rel != RelSameAs {
			n++
		}
	}
	return n
}

// World is the snapshot the groups were found in.
func (g *Groups) World() *Snapshot { return g.world }

// In is g read in s, a snapshot with the same graph as g's world, whose groups are g's though
// their members' statuses may differ; nil when g is.
func (g *Groups) In(s *Snapshot) *Groups {
	if g == nil {
		return nil
	}
	return &Groups{world: s, face: g.face, members: g.members}
}

// Len is the number of groups.
func (g *Groups) Len() int {
	if g == nil {
		return 0
	}
	return len(g.members)
}

// Face is the entity r's group shows as, or r when it is in none.
func (g *Groups) Face(r EntityRef) EntityRef {
	if g == nil {
		return r
	}
	if f, ok := g.face[r]; ok {
		return f
	}
	return r
}

// Members is r's whole group, its face first; nil when r is in none. Callers must not modify it.
func (g *Groups) Members(r EntityRef) []EntityRef {
	if g == nil {
		return nil
	}
	return g.members[g.Face(r)]
}

// Hidden reports whether r is shown as another entity, its group's face.
func (g *Groups) Hidden(r EntityRef) bool { return g.Face(r) != r }

// Worst is the member of r's group in the worst state, the earliest listed on a tie; r itself
// when it is in no group. It is nil when r is not in the world, or g is nil.
func (g *Groups) Worst(r EntityRef) *Entity {
	if g == nil {
		return nil
	}
	ms := g.Members(r)
	if ms == nil {
		e, _ := g.world.Entity(r)
		return e
	}
	var worst *Entity
	for _, m := range ms {
		if e, ok := g.world.Entity(m); ok && (worst == nil || e.Status.Level > worst.Status.Level) {
			worst = e
		}
	}
	return worst
}

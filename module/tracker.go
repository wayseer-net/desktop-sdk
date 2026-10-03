package module

import (
	"cmp"
	"encoding/binary"
	"hash/maphash"
	"math"
	"slices"
	"strconv"
	"time"
	"wayseer/pkg/sdk/model"
)

// Tracker remembers what a module has sent, so each poll sends only the difference. It keeps
// a 64-bit digest of each entity rather than a copy. It is not safe for concurrent use.
type Tracker struct {
	seed      maphash.Seed
	sent      map[model.EntityRef]uint64      // digest of the entity as sent, without Seen
	sentEdges map[model.EdgeKey]model.Traffic // an edge is resent when only its traffic changes
	buf       []byte                          // one value's bytes while digesting, reused
}

// Reset forgets what was sent, so the next Changes sends everything, as a snapshot must.
func (t *Tracker) Reset() { t.sent, t.sentEdges = nil, nil }

// Changes returns what differs between the world and what was sent, in a stable order, and
// records the world as sent. Upserts carry now as Seen.
func (t *Tracker) Changes(ents map[model.EntityRef]model.Entity, edges map[model.EdgeKey]model.Edge, now time.Time) *model.ChangeSet {
	return changes(t, ents, edges, same[model.Entity], same[model.Edge], now)
}

// ChangesOf is Changes for a world that holds its entities and edges by pointer.
func (t *Tracker) ChangesOf(ents map[model.EntityRef]*model.Entity, edges map[model.EdgeKey]*model.Edge, now time.Time) *model.ChangeSet {
	return changes(t, ents, edges, deref[model.Entity], deref[model.Edge], now)
}

func same[T any](v T) T   { return v }
func deref[T any](p *T) T { return *p }

func changes[E, D any](t *Tracker, ents map[model.EntityRef]E, edges map[model.EdgeKey]D, entity func(E) model.Entity, edge func(D) model.Edge, now time.Time) *model.ChangeSet {
	if t.sent == nil {
		t.seed = maphash.MakeSeed()
		t.sent, t.sentEdges = map[model.EntityRef]uint64{}, map[model.EdgeKey]model.Traffic{}
	}
	cs := &model.ChangeSet{}
	for r := range t.sent {
		if _, ok := ents[r]; !ok {
			cs.Removes = append(cs.Removes, r)
			delete(t.sent, r)
		}
	}
	for k := range t.sentEdges {
		if _, ok := edges[k]; !ok {
			cs.RemoveEdges = append(cs.RemoveEdges, k)
			delete(t.sentEdges, k)
		}
	}
	for r, v := range ents {
		e := entity(v)
		d := t.digest(&e)
		if old, ok := t.sent[r]; !ok || old != d {
			t.sent[r] = d
			e.Seen = now
			cs.Upserts = append(cs.Upserts, e)
		}
	}
	for k, v := range edges {
		e := edge(v)
		if old, ok := t.sentEdges[k]; !ok || old != e.Traffic {
			t.sentEdges[k] = e.Traffic
			cs.Edges = append(cs.Edges, e)
		}
	}
	sortChanges(cs)
	return cs
}

// sortChanges puts each part of cs in a stable order; only what changed is sorted, so an
// unchanged poll costs no sorting.
func sortChanges(cs *model.ChangeSet) {
	slices.Sort(cs.Removes)
	slices.SortFunc(cs.RemoveEdges, CompareEdgeKeys)
	slices.SortFunc(cs.Upserts, func(a, b model.Entity) int { return cmp.Compare(a.Ref, b.Ref) })
	slices.SortFunc(cs.Edges, func(a, b model.Edge) int { return CompareEdgeKeys(a.Key(), b.Key()) })
}

// CompareEdgeKeys orders edge keys by from, to, then relation.
func CompareEdgeKeys(a, b model.EdgeKey) int {
	return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Rel, b.Rel))
}

// digest sums up what a change would be sent for: the name, status, place, tags and attributes.
func (t *Tracker) digest(e *model.Entity) uint64 {
	var attrs uint64 // a sum, so the map's order does not matter
	for k, v := range e.Attrs {
		attrs += t.attrDigest(k, v)
	}
	t.buf = append(t.buf[:0], byte(e.Status.Level), boolByte(e.Place.Known))
	if e.Place.Known {
		t.buf = binary.LittleEndian.AppendUint32(t.buf, math.Float32bits(e.Place.Lat))
		t.buf = binary.LittleEndian.AppendUint32(t.buf, math.Float32bits(e.Place.Lon))
	}
	t.buf = appendText(appendText(t.buf, e.Name), e.Status.Reason)
	t.buf = binary.AppendUvarint(t.buf, uint64(len(e.Tags)))
	for _, tag := range e.Tags {
		t.buf = appendText(t.buf, tag)
	}
	t.buf = binary.LittleEndian.AppendUint64(t.buf, attrs)
	return maphash.Bytes(t.seed, t.buf)
}

// attrDigest digests one attribute with its value's type, so 16 and "16" differ.
func (t *Tracker) attrDigest(k string, v model.Value) uint64 {
	t.buf = appendValue(appendText(t.buf[:0], k), v)
	return maphash.Bytes(t.seed, t.buf)
}

// appendValue adds v's type, unit and text; a list's strings are quoted, so its items stay apart.
func appendValue(b []byte, v model.Value) []byte {
	b = append(append(append(b, byte(v.Type())), v.Unit()...), ':')
	if v.Type() != model.TypeList {
		return v.Append(b)
	}
	for _, item := range v.List() {
		if item.Type() == model.TypeString {
			b = strconv.AppendQuote(b, item.Str())
		} else {
			b = appendValue(b, item)
		}
		b = append(b, ',')
	}
	return b
}

// appendText adds s with its length first, so no text runs into the next.
func appendText(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

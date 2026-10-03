package model

import (
	"slices"
	"strings"
	"unicode"
)

// DefaultPlaceAttrs are the attributes read for place names when config names none.
var DefaultPlaceAttrs = []string{"region", "zone", "city"}

// Gazetteer is the owner's named places, and the attributes whose values name them.
type Gazetteer struct {
	attrs   []string
	names   map[string]Place      // by lower-case name
	display map[string]string     // each name as config writes it, by lower-case name
	at      map[[2]float32]string // the least name at each point
}

// NewGazetteer names places, matched ignoring case; attrs default to DefaultPlaceAttrs.
// Names that differ only in case keep one of their places, so callers refuse them first.
func NewGazetteer(attrs []string, names map[string]Place) *Gazetteer {
	if len(attrs) == 0 {
		attrs = DefaultPlaceAttrs
	}
	g := &Gazetteer{attrs: attrs, names: make(map[string]Place, len(names)), display: map[string]string{}, at: map[[2]float32]string{}}
	for n, p := range names {
		p.From = FromConfig
		low := strings.ToLower(n)
		g.names[low], g.display[low] = p, n
		if old, ok := g.at[p.point()]; !ok || n < old {
			g.at[p.point()] = n
		}
	}
	return g
}

// Find is the place config names name, ignoring case, and the name as config writes it.
func (g *Gazetteer) Find(name string) (string, Place, bool) {
	if g == nil {
		return "", Place{}, false
	}
	low := strings.ToLower(name)
	p, ok := g.names[low]
	return g.display[low], p, ok
}

// Names appends every name config gives, as it writes them, in order.
func (g *Gazetteer) Names(dst []string) []string {
	if g == nil {
		return dst
	}
	n := len(dst)
	for _, name := range g.display {
		dst = append(dst, name)
	}
	slices.Sort(dst[n:])
	return dst
}

// NameAt is the least name config gives p's point, wherever p came from.
func (g *Gazetteer) NameAt(p Place) (string, bool) {
	if g == nil || !p.Known {
		return "", false
	}
	n, ok := g.at[p.point()]
	return n, ok
}

// Lookup is the place named by the first of e's attributes that names one.
func (g *Gazetteer) Lookup(e *Entity) (Place, bool) {
	if g == nil {
		return Place{}, false
	}
	for _, a := range g.attrs {
		v, ok := e.Attrs[a]
		if !ok || v.Type() != TypeString {
			continue
		}
		if p, ok := g.find(v.Str()); ok {
			return p, true
		}
	}
	return Place{}, false
}

// find looks name up, lowering it only when it has upper case, as most names do not.
func (g *Gazetteer) find(name string) (Place, bool) {
	if strings.IndexFunc(name, unicode.IsUpper) >= 0 {
		name = strings.ToLower(name)
	}
	p, ok := g.names[name]
	return p, ok
}

// ownPlace is where e is by itself: the module's place, else one config names.
// A place the store resolved and a caller sent back is not the module's, so it is ignored.
func (g *Gazetteer) ownPlace(e *Entity) Place {
	if e.Place.Known && e.Place.From == FromModule {
		return e.Place
	}
	p, _ := g.Lookup(e)
	return p
}

// ownPlaced reports whether p is an entity's own place, which its members may take.
func ownPlaced(p Place) bool { return p.Known && p.From != FromMembership }

// resolvePlaces places by membership every entity whose place may have changed in t.
func (t *txn) resolvePlaces() {
	for r := range t.ents { // resolving may add keys; visiting them again is harmless
		t.placeByMembership(r)
		if t.ownPlaceChanged(r) {
			if a, ok := t.next.adj.get(r); ok {
				for _, e := range a.in {
					if e.Rel == RelMemberOf {
						t.placeByMembership(e.From)
					}
				}
			}
		}
	}
	for k := range t.edges {
		if k.Rel == RelMemberOf {
			t.placeByMembership(k.From)
		}
	}
}

// ownPlaceChanged reports whether r's own place differs from the previous snapshot's.
func (t *txn) ownPlaceChanged(r EntityRef) bool {
	var was, is Place
	if e, ok := t.prev.entities.get(r); ok && ownPlaced(e.Place) {
		was = e.Place
	}
	if e, ok := t.next.entities.get(r); ok && ownPlaced(e.Place) {
		is = e.Place
	}
	return was != is
}

// placeByMembership gives r, if it has no place of its own, the place of the first owner, by
// the order of its member_of edges, that has one of its own.
func (t *txn) placeByMembership(r EntityRef) {
	e, ok := t.next.entities.get(r)
	if !ok || ownPlaced(e.Place) {
		return
	}
	var p Place
	if a, ok := t.next.adj.get(r); ok {
		for _, ed := range a.out {
			if o, ok := t.next.entities.get(ed.To); ok && ed.Rel == RelMemberOf && ownPlaced(o.Place) {
				p, p.From = o.Place, FromMembership
				break
			}
		}
	}
	if p == e.Place {
		return
	}
	n := new(*e)
	n.Place = p
	t.next.entities = t.next.entities.set(t.tok, r, n)
	t.modifyEntity(r)
}

// SetPlaces swaps the places config names for g and places every entity again, as one
// transaction. The places modules give are kept.
func (s *Store) SetPlaces(g *Gazetteer) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.places = g
	t := s.begin()
	for e := range t.prev.Entities() {
		t.placeAgain(e)
	}
	snap := t.commit()
	s.cur.Store(snap)
	return snap
}

// placeAgain gives e the place config now names for it, unless its module placed it; places by
// membership are resolved again at commit.
func (t *txn) placeAgain(e *Entity) {
	if e.Place.Known && e.Place.From == FromModule {
		return
	}
	p, _ := t.s.places.Lookup(e)
	if p == e.Place || !p.Known && e.Place.From == FromMembership {
		return
	}
	n := new(*e)
	n.Place = p
	t.next.entities = t.next.entities.set(t.tok, e.Ref, n)
	t.modifyEntity(e.Ref)
}

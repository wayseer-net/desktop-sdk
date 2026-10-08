package model

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
)

// StoreOptions configures a Store.
type StoreOptions struct {
	EventCap int            // events retained (rounded up to a whole chunk); default 10000
	Identity []IdentityRule // rules deriving same_as edges; none by default
	Places   *Gazetteer     // places named in config; none by default
}

// Store is the world model: one writer applies change sets, any goroutine reads snapshots.
type Store struct {
	mu         sync.Mutex // serialises writers; readers never take it
	cur        atomic.Pointer[Snapshot]
	events     eventLog
	dangling   map[EdgeKey]*Edge       // edges waiting for an endpoint
	danglingBy map[EntityRef][]EdgeKey // dangling keys by endpoint; may hold stale keys
	identity   *identity               // nil without identity rules
	places     *Gazetteer              // nil without named places
}

// NewStore returns an empty store at version 0.
func NewStore(o StoreOptions) *Store {
	if o.EventCap <= 0 {
		o.EventCap = 10000
	}
	s := &Store{events: eventLog{cap: o.EventCap}, dangling: map[EdgeKey]*Edge{}, danglingBy: map[EntityRef][]EdgeKey{}, places: o.Places}
	if len(o.Identity) > 0 {
		s.identity = newIdentity(o.Identity)
	}
	s.cur.Store(&Snapshot{
		entities: newPmap[EntityRef, *Entity](hashOf), edges: newPmap[EdgeKey, *Edge](hashOf),
		adj: newPmap[EntityRef, *adjacency](hashOf), byKind: newPmap[Kind, refSet](hashOf),
		bySource: newPmap[ModuleID, refSet](hashOf), grouped: newPmap[EntityRef, struct{}](hashOf),
		shape: &shape{}, graph: &shape{},
	})
	return s
}

// Current returns the latest snapshot without blocking.
func (s *Store) Current() *Snapshot { return s.cur.Load() }

// Apply commits c as one transaction and publishes the resulting snapshot. Invalid items are
// skipped and reported in the error; everything else still applies. The store keeps c's
// attribute maps, tags and event fields, so the caller must not modify them afterwards.
func (s *Store) Apply(c *ChangeSet) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.begin()
	for _, r := range c.Removes {
		t.removeEntity(r)
	}
	for _, k := range c.RemoveEdges {
		t.removeEdge(k)
	}
	var errs []error
	for i := range c.Upserts {
		errs = append(errs, t.upsertEntity(&c.Upserts[i]))
	}
	for i := range c.Edges {
		errs = append(errs, t.upsertEdge(&c.Edges[i]))
	}
	t.reconcileIdentity()
	for i := range c.Events {
		errs = append(errs, t.appendEvent(&c.Events[i]))
	}
	snap := t.commit()
	s.cur.Store(snap)
	return snap, errors.Join(errs...)
}

// txn is one transaction's working state; its edit token lets it mutate nodes it created.
type txn struct {
	s       *Store
	tok     *edit
	prev    *Snapshot
	next    Snapshot
	ents    map[EntityRef]touch
	edges   map[EdgeKey]touch
	adjDirt map[EntityRef]bool // adjacency lists that may hold removed or duplicate keys
}

// touch records whether a key existed before the transaction and whether its value changed.
type touch struct{ before, modified bool }

func (s *Store) begin() *txn {
	prev := s.Current()
	t := &txn{
		s: s, tok: &edit{}, prev: prev, next: *prev,
		ents: map[EntityRef]touch{}, edges: map[EdgeKey]touch{}, adjDirt: map[EntityRef]bool{},
	}
	t.next.diff = Diff{}
	return t
}

// modifyEntity records that r changed in this transaction.
func (t *txn) modifyEntity(r EntityRef) {
	tc, ok := t.ents[r]
	if !ok {
		_, tc.before = t.prev.entities.get(r)
	}
	tc.modified = true
	t.ents[r] = tc
}

// modifyEdge records that k changed in this transaction.
func (t *txn) modifyEdge(k EdgeKey) {
	tc, ok := t.edges[k]
	if !ok {
		_, tc.before = t.prev.edges.get(k)
	}
	tc.modified = true
	t.edges[k] = tc
}

func (t *txn) removeEntity(r EntityRef) {
	t.dropDangling(r)
	e, ok := t.next.entities.get(r)
	if !ok {
		return
	}
	t.modifyEntity(r)
	if a, ok := t.next.adj.get(r); ok {
		for _, e := range slices.Concat(a.out, a.in) {
			t.removeEdge(e.Key())
		}
		t.next.adj = t.next.adj.del(t.tok, r)
	}
	t.next.entities = t.next.entities.del(t.tok, r)
	t.next.byKind = setIndex(t.next.byKind, t.tok, e.Kind, r, false)
	t.next.bySource = setIndex(t.next.bySource, t.tok, e.Source, r, false)
}

// dropDangling discards waiting edges that touch r.
func (t *txn) dropDangling(r EntityRef) {
	for _, k := range t.s.danglingBy[r] {
		delete(t.s.dangling, k)
	}
	delete(t.s.danglingBy, r)
}

func (t *txn) removeEdge(k EdgeKey) {
	if _, ok := t.next.edges.get(k); !ok {
		delete(t.s.dangling, k)
		return
	}
	t.modifyEdge(k)
	t.next.edges = t.next.edges.del(t.tok, k)
	t.adjDirt[k.From], t.adjDirt[k.To] = true, true
}

func (t *txn) upsertEntity(in *Entity) error {
	if err := in.Validate(); err != nil {
		return err
	}
	old, existed := t.next.entities.get(in.Ref)
	cand := *in
	cand.Place = t.s.places.ownPlace(in)
	if existed && !cand.Place.Known && old.Place.From == FromMembership {
		cand.Place = old.Place // kept until membership is resolved at commit
	}
	if existed && entityEqual(old, &cand) {
		return nil
	}
	e := new(cand)
	t.modifyEntity(e.Ref)
	t.next.entities = t.next.entities.set(t.tok, e.Ref, e)
	if !existed {
		t.next.byKind = setIndex(t.next.byKind, t.tok, e.Kind, e.Ref, true)
		t.next.bySource = setIndex(t.next.bySource, t.tok, e.Source, e.Ref, true)
		t.promote(e.Ref)
	}
	return nil
}

// promote moves waiting edges touching r into the graph once both endpoints exist.
func (t *txn) promote(r EntityRef) {
	var still []EdgeKey
	for _, k := range t.s.danglingBy[r] {
		e, ok := t.s.dangling[k]
		if !ok {
			continue
		}
		if !t.exists(k.From) || !t.exists(k.To) {
			still = append(still, k)
			continue
		}
		delete(t.s.dangling, k)
		t.addEdge(e)
	}
	if still == nil {
		delete(t.s.danglingBy, r)
	} else {
		t.s.danglingBy[r] = still
	}
}

func (t *txn) exists(r EntityRef) bool { _, ok := t.next.entities.get(r); return ok }

func (t *txn) upsertEdge(in *Edge) error {
	if err := in.Validate(); err != nil {
		return err
	}
	e := new(*in)
	k := e.Key()
	if !t.exists(k.From) || !t.exists(k.To) {
		t.s.dangling[k] = e
		t.s.danglingBy[k.From] = append(t.s.danglingBy[k.From], k)
		t.s.danglingBy[k.To] = append(t.s.danglingBy[k.To], k)
		return nil
	}
	if old, ok := t.next.edges.get(k); ok {
		if !edgeEqual(old, e) {
			t.modifyEdge(k)
			t.next.edges = t.next.edges.set(t.tok, k, e)
			t.adjDirt[k.From], t.adjDirt[k.To] = true, true // lists hold the old pointer
		}
		return nil
	}
	t.addEdge(e)
	return nil
}

func (t *txn) addEdge(e *Edge) {
	k := e.Key()
	t.modifyEdge(k)
	t.next.edges = t.next.edges.set(t.tok, k, e)
	from, to := t.adjacency(k.From), t.adjacency(k.To)
	from.out = append(from.out, e)
	to.in = append(to.in, e)
	t.adjDirt[k.From], t.adjDirt[k.To] = true, true
}

// adjacency returns r's lists, copied into this transaction on first write.
func (t *txn) adjacency(r EntityRef) *adjacency {
	a, ok := t.next.adj.get(r)
	if ok && a.owner == t.tok {
		return a
	}
	n := &adjacency{owner: t.tok}
	if ok {
		n.out, n.in = slices.Clone(a.out), slices.Clone(a.in)
	}
	t.next.adj = t.next.adj.set(t.tok, r, n)
	return n
}

func (t *txn) appendEvent(e *Event) error {
	if e.Entity != "" {
		if err := e.Entity.Validate(); err != nil {
			return fmt.Errorf("event %q: %w", e.ID, err)
		}
	}
	t.s.events.append(*e)
	t.next.diff.Events++
	return nil
}

func (t *txn) commit() *Snapshot {
	t.compactAdjacency()
	t.resolvePlaces()
	t.next.version = t.prev.version + 1
	t.next.events = t.s.events.view()
	d := &t.next.diff
	d.Added, d.Changed, d.Removed = classify(t.ents, func(r EntityRef) bool { return t.exists(r) })
	d.EdgesAdded, d.EdgesChanged, d.EdgesRemoved = classify(t.edges, func(k EdgeKey) bool { _, ok := t.next.edges.get(k); return ok })
	slices.SortFunc(d.EdgesAdded, compareEdgeKey)
	slices.SortFunc(d.EdgesChanged, compareEdgeKey)
	slices.SortFunc(d.EdgesRemoved, compareEdgeKey)
	slices.Sort(d.Added)
	slices.Sort(d.Changed)
	slices.Sort(d.Removed)
	if len(d.Added)+len(d.Changed)+len(d.Removed)+len(d.EdgesAdded)+len(d.EdgesChanged)+len(d.EdgesRemoved) > 0 {
		t.next.shape = &shape{since: t.next.version}
	}
	if len(d.Added)+len(d.Removed)+len(d.EdgesAdded)+len(d.EdgesRemoved) > 0 {
		t.next.graph = &shape{since: t.next.version}
	}
	snap := t.next // a separate allocation: &t.next would keep the txn, and so every older snapshot, alive
	return &snap
}

// compactAdjacency drops removed and duplicate keys once per touched list, keeping removal
// O(degree), and keeps the grouped index.
func (t *txn) compactAdjacency() {
	for r := range t.adjDirt {
		if !t.exists(r) {
			t.next.adj = t.next.adj.del(t.tok, r) // an absent entity has no edges
			t.markGrouped(r, false)
			continue
		}
		a := t.adjacency(r)
		a.out, a.in = t.liveEdges(a.out), t.liveEdges(a.in)
		if len(a.out)+len(a.in) == 0 {
			t.next.adj = t.next.adj.del(t.tok, r)
		}
		t.markGrouped(r, slices.ContainsFunc(a.out, isSameAs) || slices.ContainsFunc(a.in, isSameAs))
	}
}

// markGrouped adds r to the grouped index or removes it.
func (t *txn) markGrouped(r EntityRef, on bool) {
	if _, was := t.next.grouped.get(r); was == on {
		return
	}
	if on {
		t.next.grouped = t.next.grouped.set(t.tok, r, struct{}{})
	} else {
		t.next.grouped = t.next.grouped.del(t.tok, r)
	}
}

func isSameAs(e *Edge) bool { return e.Rel == RelSameAs }

// liveEdges swaps each edge for its current version, dropping removed ones and duplicates.
// Only keys touched in this transaction can repeat, so only they are tracked.
func (t *txn) liveEdges(es []*Edge) []*Edge {
	var seen map[EdgeKey]bool
	out := es[:0]
	for _, e := range es {
		k := e.Key()
		cur, ok := t.next.edges.get(k)
		if !ok {
			continue
		}
		if _, touched := t.edges[k]; touched {
			if seen[k] {
				continue
			}
			if seen == nil {
				seen = map[EdgeKey]bool{}
			}
			seen[k] = true
		}
		out = append(out, cur)
	}
	clear(es[len(out):])
	return slices.Clip(out)
}

func classify[K comparable](touched map[K]touch, exists func(K) bool) (added, changed, removed []K) {
	for k, tc := range touched {
		after := exists(k)
		switch {
		case !tc.before && after:
			added = append(added, k)
		case tc.before && !after:
			removed = append(removed, k)
		case tc.before && after && tc.modified:
			changed = append(changed, k)
		}
	}
	return added, changed, removed
}

// setIndex adds or removes r in the set stored under key in an index.
func setIndex[K comparable](idx pmap[K, refSet], tok *edit, key K, r EntityRef, add bool) pmap[K, refSet] {
	set, ok := idx.get(key)
	if !ok {
		set = newPmap[EntityRef, struct{}](hashOf)
	}
	if add {
		set = set.set(tok, r, struct{}{})
	} else {
		set = set.del(tok, r)
	}
	if set.len() == 0 {
		return idx.del(tok, key)
	}
	return idx.set(tok, key, set)
}

func compareEdgeKey(a, b EdgeKey) int {
	if c := compareStr(a.From, b.From); c != 0 {
		return c
	}
	if c := compareStr(a.To, b.To); c != 0 {
		return c
	}
	return compareStr(a.Rel, b.Rel)
}

func compareStr[T ~string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

package model

// EdgeKey identifies an edge: at most one edge of each relation joins two entities.
type EdgeKey struct {
	From, To EntityRef
	Rel      Relation
}

// Key returns the edge's identity.
func (e Edge) Key() EdgeKey { return EdgeKey{e.From, e.To, e.Rel} }

// ChangeSet is one transaction. Store.Apply applies its parts in field order: entity removals
// (with their edges), edge removals, entity upserts, edge upserts, then events.
type ChangeSet struct {
	Removes     []EntityRef
	RemoveEdges []EdgeKey
	Upserts     []Entity
	Edges       []Edge
	Events      []Event
}

// Empty reports whether c changes nothing.
func (c *ChangeSet) Empty() bool {
	return len(c.Removes)+len(c.RemoveEdges)+len(c.Upserts)+len(c.Edges)+len(c.Events) == 0
}

// Batch merges a sequence of change sets into one with the same effect when applied.
type Batch struct {
	ents    map[EntityRef]entityOp
	edges   map[EdgeKey]edgeOp
	touches map[EntityRef][]EdgeKey // batched edge upserts by endpoint
	spare   [][]EdgeKey             // emptied touches lists, for reuse
	events  []Event
	out     ChangeSet // storage ChangeSet reuses
}

type entityOp struct {
	remove bool
	upsert *Entity
}

type edgeOp struct {
	remove bool
	upsert *Edge
}

// Add merges c after everything already added. b refers into c until Reset, so c must not
// change before then.
func (b *Batch) Add(c *ChangeSet) {
	if b.ents == nil {
		b.ents, b.edges, b.touches = map[EntityRef]entityOp{}, map[EdgeKey]edgeOp{}, map[EntityRef][]EdgeKey{}
	}
	for _, r := range c.Removes {
		b.ents[r] = entityOp{remove: true}
		b.dropEdgeUpserts(r)
	}
	for _, k := range c.RemoveEdges {
		b.edges[k] = edgeOp{remove: true}
	}
	for i := range c.Upserts {
		e := &c.Upserts[i]
		op := b.ents[e.Ref]
		op.upsert = e
		b.ents[e.Ref] = op
	}
	for i := range c.Edges {
		e := &c.Edges[i]
		k := e.Key()
		op := b.edges[k]
		op.upsert = e
		b.edges[k] = op
		b.touch(k.From, k)
		b.touch(k.To, k)
	}
	b.events = append(b.events, c.Events...)
}

// dropEdgeUpserts forgets batched edges touching r, which removing r would delete anyway.
func (b *Batch) dropEdgeUpserts(r EntityRef) {
	for _, k := range b.touches[r] {
		switch op, ok := b.edges[k]; {
		case ok && op.remove:
			b.edges[k] = edgeOp{remove: true}
		case ok:
			delete(b.edges, k)
		}
	}
	if ks, ok := b.touches[r]; ok {
		b.spare = append(b.spare, ks[:0])
	}
	delete(b.touches, r)
}

// touch records that the batched edge k has r as an endpoint.
func (b *Batch) touch(r EntityRef, k EdgeKey) {
	ks, ok := b.touches[r]
	if !ok && len(b.spare) > 0 {
		ks, b.spare = b.spare[len(b.spare)-1], b.spare[:len(b.spare)-1]
	}
	b.touches[r] = append(ks, k)
}

// ChangeSet returns the merged change set.
// The result refers into b's storage, so it is valid only until the next ChangeSet call.
func (b *Batch) ChangeSet() ChangeSet {
	c := &b.out
	c.Removes, c.RemoveEdges, c.Upserts, c.Edges = c.Removes[:0], c.RemoveEdges[:0], c.Upserts[:0], c.Edges[:0]
	for r, op := range b.ents {
		if op.remove {
			c.Removes = append(c.Removes, r)
		}
		if op.upsert != nil {
			c.Upserts = append(c.Upserts, *op.upsert)
		}
	}
	for _, op := range b.edges {
		if op.upsert != nil {
			c.Edges = append(c.Edges, *op.upsert)
		}
	}
	for k, op := range b.edges {
		if op.remove {
			c.RemoveEdges = append(c.RemoveEdges, k)
		}
	}
	c.Events = append(c.Events[:0], b.events...)
	return *c
}

// keepLimit is the most entities and edges a batch may have held for Reset to keep its
// storage; a snapshot's is released, since deltas after it are small.
const keepLimit = 1 << 16

// Reset empties b, keeping its maps for reuse unless they held a snapshot.
func (b *Batch) Reset() {
	if len(b.ents)+len(b.edges) > keepLimit {
		*b = Batch{}
		return
	}
	clear(b.ents)
	clear(b.edges)
	for _, ks := range b.touches {
		b.spare = append(b.spare, ks[:0])
	}
	clear(b.touches)
	b.events = b.events[:0]
}

// Replace merges c as the whole of src's state: whatever src owns in base or earlier in b,
// and c does not declare, is removed. It scans every edge in base, so use it only for snapshots.
func (b *Batch) Replace(src ModuleID, base *Snapshot, c *ChangeSet) {
	var gone ChangeSet
	keep := make(map[EntityRef]bool, len(c.Upserts))
	for i := range c.Upserts {
		keep[c.Upserts[i].Ref] = true
	}
	for r := range base.BySource(src) {
		if !keep[r] {
			gone.Removes = append(gone.Removes, r)
		}
	}
	for r, op := range b.ents {
		if op.upsert != nil && op.upsert.Source == src && !keep[r] {
			gone.Removes = append(gone.Removes, r)
		}
	}
	gone.RemoveEdges = b.staleEdges(src, base, c)
	b.Add(&gone)
	b.Add(c)
}

// staleEdges are src's edges, in base or earlier in b, that c does not declare.
func (b *Batch) staleEdges(src ModuleID, base *Snapshot, c *ChangeSet) []EdgeKey {
	keep := make(map[EdgeKey]bool, len(c.Edges))
	for i := range c.Edges {
		keep[c.Edges[i].Key()] = true
	}
	var out []EdgeKey
	for e := range base.Edges() {
		if e.Source == src && !keep[e.Key()] {
			out = append(out, e.Key())
		}
	}
	for k, op := range b.edges {
		if op.upsert != nil && op.upsert.Source == src && !keep[k] {
			out = append(out, k)
		}
	}
	return out
}

package model

import (
	"hash/maphash"
	"iter"
	"math/bits"
	"slices"
)

// pmap is a persistent hash map (a CHAMP trie): updates return a new map sharing structure
// with the old one, so published snapshots never change and cost O(changes) to make.
type pmap[K comparable, V any] struct {
	root *hnode[K, V]
	n    int
	hash func(K) uint64
}

// edit marks nodes created within one transaction; those nodes may be mutated in place.
type edit struct{ _ [8]byte } // non-zero size so every token is a distinct pointer

type hnode[K comparable, V any] struct {
	owner     *edit
	datamap   uint32 // slots holding a leaf
	nodemap   uint32 // slots holding a subtree
	collision bool   // leaves only, searched linearly: hashes equal or trie exhausted
	leaves    []leaf[K, V]
	kids      []*hnode[K, V]
}

type leaf[K comparable, V any] struct {
	key K
	val V
}

const (
	hbits    = 5
	hmask    = 1<<hbits - 1
	maxShift = 64
)

var hashSeed = maphash.MakeSeed()

func hashOf[K comparable](k K) uint64 { return maphash.Comparable(hashSeed, k) }

func newPmap[K comparable, V any](hash func(K) uint64) pmap[K, V] {
	return pmap[K, V]{hash: hash}
}

func (m pmap[K, V]) len() int { return m.n }

func (m pmap[K, V]) get(k K) (V, bool) {
	h, n := m.hash(k), m.root
	for shift := 0; n != nil; shift += hbits {
		if n.collision {
			return findLeaf(n.leaves, k)
		}
		bit := slotBit(h, shift)
		switch {
		case n.datamap&bit != 0:
			if l := &n.leaves[index(n.datamap, bit)]; l.key == k {
				return l.val, true
			}
			n = nil
		case n.nodemap&bit != 0:
			n = n.kids[index(n.nodemap, bit)]
		default:
			n = nil
		}
	}
	var zero V
	return zero, false
}

func findLeaf[K comparable, V any](ls []leaf[K, V], k K) (V, bool) {
	for i := range ls {
		if ls[i].key == k {
			return ls[i].val, true
		}
	}
	var zero V
	return zero, false
}

func slotBit(h uint64, shift int) uint32 { return 1 << (h >> shift & hmask) }

func index(bitmap, bit uint32) int { return bits.OnesCount32(bitmap & (bit - 1)) }

// set returns m with k bound to v; nodes owned by tok are edited in place.
func (m pmap[K, V]) set(tok *edit, k K, v V) pmap[K, V] {
	var added bool
	m.root, added = m.root.set(m.hash, tok, 0, m.hash(k), leaf[K, V]{k, v})
	if added {
		m.n++
	}
	return m
}

// del returns m without k.
func (m pmap[K, V]) del(tok *edit, k K) pmap[K, V] {
	root, removed := m.root.del(m.hash, tok, 0, m.hash(k), k)
	if removed {
		m.root = collapseRoot(m.hash, root)
		m.n--
	}
	return m
}

// all yields every entry in unspecified order.
func (m pmap[K, V]) all() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { m.root.each(yield) }
}

// editable returns n itself if tok owns it, else an exact-size copy owned by tok.
func (n *hnode[K, V]) editable(tok *edit) *hnode[K, V] {
	if tok != nil && n.owner == tok {
		return n
	}
	c := *n
	c.owner = tok
	c.leaves = slices.Clip(slices.Clone(n.leaves))
	c.kids = slices.Clip(slices.Clone(n.kids))
	return &c
}

func (n *hnode[K, V]) set(hash func(K) uint64, tok *edit, shift int, h uint64, l leaf[K, V]) (*hnode[K, V], bool) {
	if n == nil {
		return &hnode[K, V]{owner: tok, datamap: slotBit(h, shift), leaves: []leaf[K, V]{l}}, true
	}
	if n.collision {
		return n.setCollision(tok, l)
	}
	bit := slotBit(h, shift)
	switch {
	case n.datamap&bit != 0:
		i := index(n.datamap, bit)
		if n.leaves[i].key == l.key {
			e := n.editable(tok)
			e.leaves[i].val = l.val
			return e, false
		}
		old := n.leaves[i]
		kid := pair(tok, shift+hbits, old, hash(old.key), l, h)
		e := n.editable(tok)
		e.datamap &^= bit
		e.leaves = slices.Delete(e.leaves, i, i+1)
		e.nodemap |= bit
		e.kids = insertAt(e.kids, index(e.nodemap, bit), kid)
		return e, true
	case n.nodemap&bit != 0:
		j := index(n.nodemap, bit)
		kid, added := n.kids[j].set(hash, tok, shift+hbits, h, l)
		e := n.editable(tok)
		e.kids[j] = kid
		return e, added
	}
	e := n.editable(tok)
	e.datamap |= bit
	e.leaves = insertAt(e.leaves, index(e.datamap, bit), l)
	return e, true
}

func (n *hnode[K, V]) setCollision(tok *edit, l leaf[K, V]) (*hnode[K, V], bool) {
	e := n.editable(tok)
	for i := range e.leaves {
		if e.leaves[i].key == l.key {
			e.leaves[i].val = l.val
			return e, false
		}
	}
	e.leaves = insertAt(e.leaves, len(e.leaves), l)
	return e, true
}

// insertAt returns s with v at i in a new exact-size array, keeping trie nodes compact.
func insertAt[T any](s []T, i int, v T) []T {
	out := make([]T, len(s)+1)
	copy(out, s[:i])
	out[i] = v
	copy(out[i+1:], s[i:])
	return out
}

// pair builds the smallest subtree holding two leaves with different keys.
func pair[K comparable, V any](tok *edit, shift int, a leaf[K, V], ha uint64, b leaf[K, V], hb uint64) *hnode[K, V] {
	if shift >= maxShift || ha == hb {
		return &hnode[K, V]{owner: tok, collision: true, leaves: []leaf[K, V]{a, b}}
	}
	ba, bb := slotBit(ha, shift), slotBit(hb, shift)
	if ba == bb {
		return &hnode[K, V]{owner: tok, nodemap: ba, kids: []*hnode[K, V]{pair(tok, shift+hbits, a, ha, b, hb)}}
	}
	if ba > bb {
		a, b = b, a
	}
	return &hnode[K, V]{owner: tok, datamap: ba | bb, leaves: []leaf[K, V]{a, b}}
}

func (n *hnode[K, V]) del(hash func(K) uint64, tok *edit, shift int, h uint64, k K) (*hnode[K, V], bool) {
	if n == nil {
		return nil, false
	}
	if n.collision {
		i := slices.IndexFunc(n.leaves, func(l leaf[K, V]) bool { return l.key == k })
		if i < 0 {
			return n, false
		}
		e := n.editable(tok)
		e.leaves = slices.Delete(e.leaves, i, i+1)
		return e, true
	}
	bit := slotBit(h, shift)
	switch {
	case n.datamap&bit != 0:
		i := index(n.datamap, bit)
		if n.leaves[i].key != k {
			return n, false
		}
		e := n.editable(tok)
		e.datamap &^= bit
		e.leaves = slices.Delete(e.leaves, i, i+1)
		return e, true
	case n.nodemap&bit != 0:
		j := index(n.nodemap, bit)
		kid, removed := n.kids[j].del(hash, tok, shift+hbits, h, k)
		if !removed {
			return n, false
		}
		e := n.editable(tok)
		if l, ok := soleLeaf(kid); ok {
			e.nodemap &^= bit
			e.kids = slices.Delete(e.kids, j, j+1)
			e.datamap |= bit
			e.leaves = insertAt(e.leaves, index(e.datamap, bit), l)
		} else {
			e.kids[j] = kid
		}
		return e, true
	}
	return n, false
}

// soleLeaf reports whether n has shrunk to one leaf, which can then replace it in its parent.
func soleLeaf[K comparable, V any](n *hnode[K, V]) (leaf[K, V], bool) {
	if len(n.leaves) == 1 && len(n.kids) == 0 {
		return n.leaves[0], true
	}
	return leaf[K, V]{}, false
}

// collapseRoot drops an empty root and turns a one-leaf collision root back into a normal node.
func collapseRoot[K comparable, V any](hash func(K) uint64, n *hnode[K, V]) *hnode[K, V] {
	switch {
	case n == nil || len(n.leaves)+len(n.kids) == 0:
		return nil
	case n.collision && len(n.leaves) == 1:
		l := n.leaves[0]
		return &hnode[K, V]{datamap: slotBit(hash(l.key), 0), leaves: []leaf[K, V]{l}}
	}
	return n
}

func (n *hnode[K, V]) each(yield func(K, V) bool) bool {
	if n == nil {
		return true
	}
	for i := range n.leaves {
		if !yield(n.leaves[i].key, n.leaves[i].val) {
			return false
		}
	}
	for _, k := range n.kids {
		if !k.each(yield) {
			return false
		}
	}
	return true
}

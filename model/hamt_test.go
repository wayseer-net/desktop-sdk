package model

import (
	"math/rand/v2"
	"testing"
)

func intMap(hash func(int) uint64) pmap[int, int] { return newPmap[int, int](hash) }

// checkEqual asserts m holds exactly want.
func checkEqual(t *testing.T, m pmap[int, int], want map[int]int) {
	t.Helper()
	if m.len() != len(want) {
		t.Fatalf("len = %d, want %d", m.len(), len(want))
	}
	for k, v := range want {
		if got, ok := m.get(k); !ok || got != v {
			t.Fatalf("get(%d) = %d, %v; want %d", k, got, ok, v)
		}
	}
	seen := 0
	for k, v := range m.all() {
		if want[k] != v {
			t.Fatalf("all yielded %d=%d, want %d", k, v, want[k])
		}
		seen++
	}
	if seen != len(want) {
		t.Fatalf("all yielded %d entries, want %d", seen, len(want))
	}
}

// randomOps mutates m and ref identically; tok may be nil (persistent) or an edit token.
func randomOps(r *rand.Rand, m pmap[int, int], ref map[int]int, tok *edit, n, keys int) pmap[int, int] {
	for range n {
		k := r.IntN(keys)
		if r.IntN(3) == 0 {
			m = m.del(tok, k)
			delete(ref, k)
		} else {
			v := r.Int()
			m = m.set(tok, k, v)
			ref[k] = v
		}
	}
	return m
}

func TestPmapMatchesBuiltinMap(t *testing.T) {
	hashes := map[string]func(int) uint64{
		"good":      func(k int) uint64 { return hashOf(k) },
		"collide":   func(k int) uint64 { return uint64(k % 7) },   // deep collision nodes
		"lowbits":   func(k int) uint64 { return uint64(k) << 40 }, // shares long prefixes
		"identical": func(int) uint64 { return 42 },                // one collision list
	}
	for name, h := range hashes {
		t.Run(name, func(t *testing.T) {
			r := rand.New(rand.NewPCG(1, 2))
			ref := map[int]int{}
			m := randomOps(r, intMap(h), ref, nil, 5000, 300)
			checkEqual(t, m, ref)
			m = randomOps(r, m, ref, &edit{}, 5000, 300)
			checkEqual(t, m, ref)
		})
	}
}

func TestPmapPersistence(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	ref := map[int]int{}
	base := randomOps(r, intMap(func(k int) uint64 { return hashOf(k) }), ref, nil, 2000, 500)
	frozen := map[int]int{}
	for k, v := range ref {
		frozen[k] = v
	}
	// Both persistent and transient edits must leave base untouched.
	_ = randomOps(r, base, map[int]int{}, nil, 2000, 500)
	_ = randomOps(r, base, map[int]int{}, &edit{}, 2000, 500)
	checkEqual(t, base, frozen)
}

func TestPmapTransientReusesOwnedNodes(t *testing.T) {
	tok := &edit{}
	m := intMap(func(k int) uint64 { return hashOf(k) })
	for i := range 1000 {
		m = m.set(tok, i, i)
	}
	root := m.root
	m = m.set(tok, 5, 50)
	if m.root != root {
		t.Error("an owned root should be edited in place")
	}
	m2 := m.set(nil, 5, 51)
	if m2.root == m.root {
		t.Error("a persistent set must copy the root")
	}
	if v, _ := m.get(5); v != 50 {
		t.Errorf("original changed to %d", v)
	}
}

func TestPmapDeleteCollapses(t *testing.T) {
	m := intMap(func(int) uint64 { return 42 })
	m = m.set(nil, 1, 1).set(nil, 2, 2).del(nil, 1).del(nil, 3)
	checkEqual(t, m, map[int]int{2: 2})
	m = m.del(nil, 2)
	if m.root != nil || m.len() != 0 {
		t.Errorf("empty map should have no root, len %d", m.len())
	}
}

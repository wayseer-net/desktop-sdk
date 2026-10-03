package model

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/quick"
)

// changes is a random sequence of change sets over a five-entity universe.
type changes []ChangeSet

var universe = func() []Entity {
	var es []Entity
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		r, _ := NewEntityRef("lh", KindHost, n)
		es = append(es, Entity{Ref: r, Kind: KindHost, Name: n, Source: "lh"})
	}
	return es
}()

var relations = []Relation{RelTalksTo, RelDependsOn}

func (changes) Generate(r *rand.Rand, size int) reflect.Value {
	seq := make(changes, 1+r.Intn(size+1))
	for i := range seq {
		seq[i] = randomChangeSet(r)
	}
	return reflect.ValueOf(seq)
}

func randomChangeSet(r *rand.Rand) ChangeSet {
	var c ChangeSet
	pick := func() Entity { return universe[r.Intn(len(universe))] }
	for range r.Intn(3) {
		c.Removes = append(c.Removes, pick().Ref)
	}
	for range r.Intn(3) {
		c.RemoveEdges = append(c.RemoveEdges, randomEdge(r, pick).Key())
	}
	for range r.Intn(4) {
		e := pick()
		e.Name += []string{"", "'"}[r.Intn(2)]
		c.Upserts = append(c.Upserts, e)
	}
	for range r.Intn(5) {
		c.Edges = append(c.Edges, randomEdge(r, pick))
	}
	if r.Intn(3) == 0 {
		c.Events = append(c.Events, Event{ID: fmt.Sprint(r.Int())})
	}
	return c
}

func randomEdge(r *rand.Rand, pick func() Entity) Edge {
	a, b := pick(), pick()
	for a.Ref == b.Ref {
		b = pick()
	}
	return Edge{From: a.Ref, To: b.Ref, Rel: relations[r.Intn(len(relations))], Weight: float64(r.Intn(2)), Source: "lh"}
}

// dump renders everything a snapshot exposes, in a canonical order.
func dump(s *Snapshot) string {
	var b strings.Builder
	for _, e := range sorted(s.Entities(), func(a, b *Entity) int { return compareStr(a.Ref, b.Ref) }) {
		fmt.Fprintf(&b, "E %s %s\n", e.Ref, e.Name)
		for _, x := range sorted(s.Out(e.Ref), func(a, b *Edge) int { return compareEdgeKey(a.Key(), b.Key()) }) {
			fmt.Fprintf(&b, "  out %v %v\n", x.Key(), x.Weight)
		}
		for _, x := range sorted(s.In(e.Ref), func(a, b *Edge) int { return compareEdgeKey(a.Key(), b.Key()) }) {
			fmt.Fprintf(&b, "  in %v\n", x.Key())
		}
	}
	fmt.Fprintf(&b, "edges %d kind %v source %v events %d\n", s.EdgeLen(),
		refs(s.ByKind(KindHost)), refs(s.BySource("lh")), len(slices.Collect(s.Events())))
	return b.String()
}

// reveal upserts the whole universe, exposing dangling edges in the dump.
func reveal(t *testing.T, s *Store) string {
	snap, err := s.Apply(&ChangeSet{Upserts: universe})
	if err != nil {
		t.Fatal(err)
	}
	return dump(snap)
}

func TestSequenceEqualsMergedBatch(t *testing.T) {
	prop := func(seq changes) bool {
		stepwise, merged := NewStore(StoreOptions{}), NewStore(StoreOptions{})
		var b Batch
		for i := range seq {
			if _, err := stepwise.Apply(&seq[i]); err != nil {
				t.Fatal(err)
			}
			b.Add(&seq[i])
		}
		cs := b.ChangeSet()
		if _, err := merged.Apply(&cs); err != nil {
			t.Fatal(err)
		}
		if got, want := dump(merged.Current()), dump(stepwise.Current()); got != want {
			t.Logf("merged:\n%s\nstepwise:\n%s", got, want)
			return false
		}
		return reveal(t, merged) == reveal(t, stepwise)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}

func TestReadersRaceWriter(t *testing.T) {
	s := NewStore(StoreOptions{EventCap: 300})
	r := rand.New(rand.NewSource(7))
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = dump(s.Current())
			}
		})
	}
	for range 3000 {
		c := randomChangeSet(r)
		if _, err := s.Apply(&c); err != nil {
			t.Error(err)
		}
	}
	close(stop)
	wg.Wait()
}

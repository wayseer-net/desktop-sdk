package model

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"
	"time"
)

const (
	benchEntities = 250_000
	benchEdges    = 1_000_000
	benchBatch    = 10_000
)

func benchEntity(i int) Entity {
	kind := []Kind{KindHost, KindService, KindProcess, KindDisk}[i%4]
	r, _ := NewEntityRef("bench", kind, fmt.Sprintf("entity-%07d", i))
	return Entity{
		Ref: r, Kind: kind, Name: fmt.Sprintf("entity-%d", i), Source: "bench", Seen: time.Unix(int64(i), 0),
		Attrs: map[string]Value{"cpu": Number(float64(i % 100)), "zone": String("eu-1"), "up": Bool(true), "since": Time(time.Unix(0, 0))},
	}
}

// buildWorld loads the benchmark world in coalescer-sized transactions.
func buildWorld(b *testing.B) (*Store, []EntityRef) { return buildWorldBatched(b, benchBatch) }

// buildWorldBatched loads the benchmark world in transactions of at most batch items.
func buildWorldBatched(b *testing.B, batch int) (*Store, []EntityRef) {
	s := NewStore(StoreOptions{})
	refs := make([]EntityRef, benchEntities)
	var c ChangeSet
	for i := range benchEntities {
		e := benchEntity(i)
		refs[i] = e.Ref
		c.Upserts = append(c.Upserts, e)
		if len(c.Upserts) == batch {
			mustApply(b, s, &c)
			c.Upserts = c.Upserts[:0]
		}
	}
	r := rand.New(rand.NewPCG(1, 1))
	for i := range benchEdges {
		from := refs[i%benchEntities]
		to := refs[r.IntN(benchEntities)]
		if to == from {
			continue
		}
		c.Edges = append(c.Edges, Edge{From: from, To: to, Rel: RelDependsOn, Weight: 1, Source: "bench"})
		if len(c.Edges) == batch {
			mustApply(b, s, &c)
			c.Edges = c.Edges[:0]
		}
	}
	mustApply(b, s, &c)
	return s, refs
}

func mustApply(tb testing.TB, s *Store, c *ChangeSet) *Snapshot {
	tb.Helper()
	snap, err := s.Apply(c)
	if err != nil {
		tb.Fatal(err)
	}
	return snap
}

func heapMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func BenchmarkStoreBuild250k1M(b *testing.B) {
	for b.Loop() {
		base := heapMB()
		s, _ := buildWorld(b)
		b.ReportMetric(heapMB()-base, "heap-MB")
		snap := s.Current()
		if snap.Len() != benchEntities || snap.EdgeLen() < benchEdges*99/100 {
			b.Fatalf("built %d entities, %d edges", snap.Len(), snap.EdgeLen())
		}
		runtime.KeepAlive(s)
	}
}

// BenchmarkStoreBulk250k1M loads the world as one transaction, as a module's first snapshot arrives.
func BenchmarkStoreBulk250k1M(b *testing.B) {
	for b.Loop() {
		base := heapMB()
		s, _ := buildWorldBatched(b, benchEntities+benchEdges)
		b.ReportMetric(heapMB()-base, "heap-MB")
		runtime.KeepAlive(s)
	}
}

// BenchmarkStoreDelta is one frame's worth of changes into the full world.
func BenchmarkStoreDelta(b *testing.B) {
	s, refs := buildWorld(b)
	r := rand.New(rand.NewPCG(2, 2))
	pool := make([]Entity, 1000)
	for i := range pool {
		pool[i] = benchEntity(r.IntN(len(refs)))
	}
	c := ChangeSet{Upserts: pool}
	for i := 0; b.Loop(); i++ {
		for j := range pool {
			pool[j].Seen = time.Unix(int64(i), 1) // every upsert is a real change
		}
		mustApply(b, s, &c)
	}
}

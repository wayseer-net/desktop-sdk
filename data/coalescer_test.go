package data

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"wayseer.dev/sdk/model"
)

func testEntity(src model.ModuleID, native string, cpu float64) model.Entity {
	r, err := model.NewEntityRef(string(src), model.KindHost, native)
	if err != nil {
		panic(err)
	}
	return model.Entity{
		Ref: r, Kind: model.KindHost, Name: native, Source: src,
		Attrs: map[string]model.Value{"cpu": model.Number(cpu)},
	}
}

func newTestCoalescer(buf int, now func() time.Time) (*Coalescer, *model.Store, *[]*model.Snapshot) {
	store := model.NewStore(model.StoreOptions{})
	var commits []*model.Snapshot
	c := NewCoalescer(store, CoalescerOptions{
		Buffer: buf, Now: now,
		Publish: func(s *model.Snapshot) { commits = append(commits, s) },
	})
	return c, store, &commits
}

func TestCoalescerManyWritersOneCommit(t *testing.T) {
	const writers, perWriter = 8, 1250
	c, _, commits := newTestCoalescer(writers*perWriter, time.Now)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			src := model.ModuleID(fmt.Sprintf("m%d", w))
			for i := range perWriter {
				e := testEntity(src, fmt.Sprintf("h%d", i%100), float64(i))
				if err := c.Submit(context.Background(), src, &model.ChangeSet{Upserts: []model.Entity{e}}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	snap, err := c.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if len(*commits) != 1 || (*commits)[0] != snap {
		t.Fatalf("commits = %d, want 1", len(*commits))
	}
	if snap.Len() != writers*100 {
		t.Fatalf("entities = %d, want %d", snap.Len(), writers*100)
	}
	for w := range writers {
		// Each writer's last upsert of h99 carried the last i with i%100 == 99.
		e, ok := snap.Entity(testEntity(model.ModuleID(fmt.Sprintf("m%d", w)), "h99", 0).Ref)
		if !ok || e.Attrs["cpu"].Num() != (perWriter-1)/100*100-1 {
			t.Errorf("m%d h99 = %+v, %v", w, e, ok)
		}
	}
}

func TestCoalescerRemoveAfterUpsertRemoves(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	e := testEntity("m", "a", 1)
	ctx := context.Background()
	_ = c.Submit(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{e}})
	_ = c.Submit(ctx, "m", &model.ChangeSet{Removes: []model.EntityRef{e.Ref}})
	snap, err := c.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Entity(e.Ref); ok {
		t.Error("entity survived a later remove")
	}
}

func TestCoalescerEmptyFlushDoesNotCommit(t *testing.T) {
	c, store, commits := newTestCoalescer(8, time.Now)
	before := store.Current()
	snap, err := c.Flush()
	if err != nil || snap != before || len(*commits) != 0 {
		t.Errorf("empty flush: snap changed=%v err=%v commits=%d", snap != before, err, len(*commits))
	}
}

func TestCoalescerBackPressureCountedNotBlockingFlush(t *testing.T) {
	c, _, _ := newTestCoalescer(2, time.Now)
	ctx := context.Background()
	for i := range 2 {
		_ = c.Submit(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{testEntity("m", fmt.Sprint(i), 0)}})
	}
	blocked := make(chan error)
	go func() {
		blocked <- c.Submit(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{testEntity("m", "x", 0)}})
	}()
	waitFor(t, func() bool { return c.Stats().Waits == 1 })

	done := make(chan struct{})
	go func() { _, _ = c.Flush(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Flush blocked behind a full queue")
	}
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
}

func TestCoalescerSubmitHonoursCancellation(t *testing.T) {
	c, _, _ := newTestCoalescer(1, time.Now)
	_ = c.Submit(context.Background(), "m", &model.ChangeSet{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Submit(ctx, "m", &model.ChangeSet{}); err == nil {
		t.Fatal("Submit to a full queue ignored cancellation")
	}
	if st := c.Stats(); st.Waits != 1 || st.Abandoned != 1 {
		t.Errorf("stats = %+v", st)
	}
}

func TestCoalescerRejectsForeignOrInvalidChanges(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	ctx := context.Background()
	other := testEntity("other", "a", 1)
	bad := testEntity("m", "b", 1)
	bad.Kind = "service" // disagrees with the ref
	cases := map[string]*model.ChangeSet{
		"foreign upsert": {Upserts: []model.Entity{other}},
		"invalid upsert": {Upserts: []model.Entity{bad}},
		"foreign remove": {Removes: []model.EntityRef{other.Ref}},
		"foreign edge":   {Edges: []model.Edge{{From: bad.Ref, To: other.Ref, Rel: model.RelDependsOn, Source: "other"}}},
		"foreign event":  {Events: []model.Event{{ID: "e", Source: "other"}}},
	}
	for name, cs := range cases {
		if err := c.Submit(ctx, "m", cs); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if st := c.Stats(); st.Rejected != uint64(len(cases)) || st.Submitted != 0 {
		t.Errorf("stats = %+v", st)
	}
}

func TestCoalescerMarksSourcesSeen(t *testing.T) {
	now := time.Unix(1000, 0)
	c, _, _ := newTestCoalescer(8, func() time.Time { return now })
	_ = c.Submit(context.Background(), "m", &model.ChangeSet{})
	if _, err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := c.Freshness().State("m", now); got != FreshLive {
		t.Errorf("state = %v, want live", got)
	}
}

func TestCoalescerEmptyDeltasKeepAModuleLiveWithoutCommitting(t *testing.T) {
	now := time.Unix(1000, 0)
	c, store, commits := newTestCoalescer(8, func() time.Time { return now })
	before := store.Current()
	for range 3 {
		now = now.Add(20 * time.Second)
		mustSubmit(t, c.Submit(context.Background(), "m", &model.ChangeSet{}))
		snap, err := c.Flush()
		if err != nil || snap != before || len(*commits) != 0 {
			t.Fatalf("empty deltas: snap changed=%v err=%v commits=%d", snap != before, err, len(*commits))
		}
	}
	if got := c.Freshness().State("m", now.Add(20*time.Second)); got != FreshLive {
		t.Errorf("state = %v, want live", got)
	}
}

func TestCoalescerSnapshotReplacesTheModulesState(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	ctx := context.Background()
	old, other := testEntity("m", "old", 1), testEntity("x", "keep", 1)
	mustSubmit(t, c.Submit(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{old}}))
	mustSubmit(t, c.Submit(ctx, "x", &model.ChangeSet{Upserts: []model.Entity{other}}))
	if _, err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	queued, fresh := testEntity("m", "queued", 1), testEntity("m", "new", 1)
	mustSubmit(t, c.Submit(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{queued}}))
	mustSubmit(t, c.SubmitSnapshot(ctx, "m", &model.ChangeSet{Upserts: []model.Entity{fresh}}))
	snap, err := c.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Len() != 2 {
		t.Errorf("entities = %d, want the snapshot's one plus x's", snap.Len())
	}
	for _, e := range []model.Entity{fresh, other} {
		if _, ok := snap.Entity(e.Ref); !ok {
			t.Errorf("%s missing", e.Ref)
		}
	}
}

func TestCoalescerSnapshotChecksOwnership(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	cs := &model.ChangeSet{Upserts: []model.Entity{testEntity("x", "h", 1)}}
	if err := c.SubmitSnapshot(context.Background(), "m", cs); !errors.Is(err, ErrForeign) {
		t.Errorf("err = %v, want ErrForeign", err)
	}
}

func mustSubmit(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 1s")
		}
		time.Sleep(time.Millisecond)
	}
}

func coalesceBench() (*Coalescer, []model.ChangeSet) {
	c, _, _ := newTestCoalescer(10_000, time.Now)
	sets := make([]model.ChangeSet, 10_000)
	for i := range sets {
		sets[i].Upserts = []model.Entity{testEntity("m", fmt.Sprintf("h%d", i%2000), 0)}
	}
	return c, sets
}

// submitAll queues a fresh value for every set, so each flush changes 2000 entities.
func submitAll(b *testing.B, c *Coalescer, sets []model.ChangeSet, round int) {
	for j := range sets {
		sets[j].Upserts[0].Attrs = map[string]model.Value{"cpu": model.Number(float64(round))}
		if err := c.Submit(context.Background(), "m", &sets[j]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCoalesceFlush10k times one Flush of 10k queued updates, as the frame loop sees it.
func BenchmarkCoalesceFlush10k(b *testing.B) {
	c, sets := coalesceBench()
	for i := 0; b.Loop(); i++ {
		b.StopTimer()
		submitAll(b, c, sets, i)
		b.StartTimer()
		if _, err := c.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCoalesceSubmit10k times the module side: validating and queueing 10k updates.
func BenchmarkCoalesceSubmit10k(b *testing.B) {
	c, sets := coalesceBench()
	for i := 0; b.Loop(); i++ {
		submitAll(b, c, sets, i)
		b.StopTimer()
		if _, err := c.Flush(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func TestRemovingASource(t *testing.T) {
	c, _, _ := newTestCoalescer(16, time.Now)
	ctx := context.Background()
	a1, a2, b1 := testEntity("a", "a1", 1), testEntity("a", "a2", 1), testEntity("b", "b1", 1)
	own := model.Edge{From: a1.Ref, To: a2.Ref, Rel: model.RelTalksTo, Source: "a"}
	across := model.Edge{From: b1.Ref, To: a1.Ref, Rel: model.RelDependsOn, Source: "b"}
	mustSubmit(t, c.SubmitSnapshot(ctx, "a", &model.ChangeSet{Upserts: []model.Entity{a1, a2}, Edges: []model.Edge{own}}))
	mustSubmit(t, c.SubmitSnapshot(ctx, "b", &model.ChangeSet{Upserts: []model.Entity{b1}, Edges: []model.Edge{across}}))
	before, _ := c.Flush()
	mustSubmit(t, c.RemoveSource(ctx, "a"))
	snap, err := c.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version() == before.Version() {
		t.Fatal("removing a source should commit one change")
	}
	for _, r := range []model.EntityRef{a1.Ref, a2.Ref} {
		if _, ok := snap.Entity(r); ok {
			t.Errorf("%s survived its source's removal", r)
		}
	}
	if _, ok := snap.Entity(b1.Ref); !ok {
		t.Error("another source's entity went too")
	}
	if snap.EdgeLen() != 0 {
		t.Errorf("edges = %d, want 0: both touch the removed source", snap.EdgeLen())
	}
	if got := c.Freshness().Modules(); len(got) != 1 || got[0] != "b" {
		t.Errorf("freshness modules = %v, want [b]", got)
	}
}

func TestRemovingASourceThenSendingAgainInOneFlush(t *testing.T) {
	c, _, _ := newTestCoalescer(16, time.Now)
	ctx := context.Background()
	old, fresh := testEntity("a", "old", 1), testEntity("a", "new", 1)
	mustSubmit(t, c.SubmitSnapshot(ctx, "a", &model.ChangeSet{Upserts: []model.Entity{old}}))
	mustSubmit(t, c.RemoveSource(ctx, "a"))
	mustSubmit(t, c.SubmitSnapshot(ctx, "a", &model.ChangeSet{Upserts: []model.Entity{fresh}}))
	snap, _ := c.Flush()
	if _, ok := snap.Entity(old.Ref); ok {
		t.Error("the removed run's entity survived")
	}
	if _, ok := snap.Entity(fresh.Ref); !ok {
		t.Error("the next run's entity is missing")
	}
	if got := c.Freshness().State("a", time.Now()); got != FreshLive {
		t.Errorf("state = %v, want live: the source sent after its removal", got)
	}
}

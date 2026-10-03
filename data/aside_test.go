package data

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"wayseer/pkg/sdk/model"
)

func asideCoalescer(aside int) (*Coalescer, *[]*model.Snapshot) {
	store := model.NewStore(model.StoreOptions{})
	var commits []*model.Snapshot
	c := NewCoalescer(store, CoalescerOptions{
		Aside: aside, Publish: func(s *model.Snapshot) { commits = append(commits, s) },
	})
	return c, &commits
}

func hosts(src model.ModuleID, n int, cpu float64) *model.ChangeSet {
	cs := &model.ChangeSet{}
	for i := range n {
		cs.Upserts = append(cs.Upserts, testEntity(src, fmt.Sprintf("h%d", i), cpu))
	}
	return cs
}

// flushUntil flushes until the world has n entities, and returns how many flushes it took.
func flushUntil(t *testing.T, c *Coalescer, n int) (*model.Snapshot, int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for flushes := 1; ; flushes++ {
		snap, err := c.Flush()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Len() == n {
			return snap, flushes
		}
		if time.Now().After(deadline) {
			t.Fatalf("the world has %d entities after %d flushes; want %d", snap.Len(), flushes, n)
		}
		time.Sleep(time.Millisecond)
	}
}

func flush(t *testing.T, c *Coalescer) {
	t.Helper()
	if _, err := c.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestALargeFlushAppliesAsideAndLandsOnALaterFlush(t *testing.T) {
	c, commits := asideCoalescer(100)
	if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 1000, 1)); err != nil {
		t.Fatal(err)
	}
	snap, err := c.Flush()
	if err != nil || snap.Len() != 0 || len(*commits) != 0 {
		t.Fatalf("the first flush: %d entities, %d commits, %v; want the world before it", snap.Len(), len(*commits), err)
	}
	if _, flushes := flushUntil(t, c, 1000); flushes < 1 || len(*commits) != 1 {
		t.Errorf("%d commits published; want one", len(*commits))
	}
	if got := c.Freshness().State("m", time.Now()); got != FreshLive {
		t.Errorf("m is %v once its data landed; want live", got)
	}
}

func TestAFlushNeverReturnsAnAsideCommitBeforeItLands(t *testing.T) {
	for range 200 {
		c, commits := asideCoalescer(100)
		if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 1000, 1)); err != nil {
			t.Fatal(err)
		}
		flush(t, c)
		for c.store.Current().Len() == 0 {
			runtime.Gosched()
		}
		if snap, _ := c.Flush(); snap.Len() != 0 && len(*commits) != 1 {
			t.Fatalf("a flush returned the aside commit with %d commits published; want it published first", len(*commits))
		}
		if _, err := c.Settle(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestASmallFlushStillAppliesAtOnce(t *testing.T) {
	c, _ := asideCoalescer(100)
	if err := c.Submit(t.Context(), "m", hosts("m", 99, 1)); err != nil {
		t.Fatal(err)
	}
	if snap, _ := c.Flush(); snap.Len() != 99 {
		t.Errorf("%d entities after one flush; want 99", snap.Len())
	}
}

func TestReplacingALargeSourceWithLittleCountsWhatItRemoves(t *testing.T) {
	c, _ := asideCoalescer(100)
	if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 200, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveSource(t.Context(), "m"); err != nil {
		t.Fatal(err)
	}
	if snap, _ := c.Flush(); snap.Len() != 200 {
		t.Errorf("%d entities after the removal's first flush; want it applied aside", snap.Len())
	}
	flushUntil(t, c, 0)
}

func TestChangesAfterALargeOneApplyAfterIt(t *testing.T) {
	c, _ := asideCoalescer(100)
	if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 1000, 1)); err != nil {
		t.Fatal(err)
	}
	flush(t, c)
	if err := c.Submit(t.Context(), "m", hosts("m", 1, 99)); err != nil {
		t.Fatal(err)
	}
	if err := c.Submit(t.Context(), "n", hosts("n", 1, 5)); err != nil {
		t.Fatal(err)
	}
	snap, _ := flushUntil(t, c, 1001)
	h0 := hosts("m", 1, 0).Upserts[0].Ref
	if e, _ := snap.Entity(h0); e.Attrs["cpu"] != model.Number(99) {
		t.Errorf("h0's cpu is %v; want the later change's 99", e.Attrs["cpu"])
	}
	if err := c.RemoveSource(t.Context(), "m"); err != nil {
		t.Fatal(err)
	}
	flushUntil(t, c, 1)
}

func TestSettleLandsEverything(t *testing.T) {
	c, commits := asideCoalescer(100)
	if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 1000, 1)); err != nil {
		t.Fatal(err)
	}
	flush(t, c)
	if err := c.SubmitSnapshot(t.Context(), "n", hosts("n", 500, 1)); err != nil {
		t.Fatal(err)
	}
	snap, err := c.Settle()
	if err != nil || snap.Len() != 1500 || len(*commits) != 2 {
		t.Errorf("settled at %d entities in %d commits, %v; want 1500 in 2", snap.Len(), len(*commits), err)
	}
	if err := c.SubmitSnapshot(t.Context(), "o", hosts("o", 500, 1)); err != nil {
		t.Fatal(err)
	}
	if snap, _ := c.Flush(); snap.Len() != 1500 {
		t.Errorf("a flush after settling applied at once; want large ones still aside")
	}
}

func TestSetPlacesWaitsForAChangeAppliedAside(t *testing.T) {
	c, _ := asideCoalescer(100)
	if err := c.SubmitSnapshot(t.Context(), "m", hosts("m", 1000, 1)); err != nil {
		t.Fatal(err)
	}
	flush(t, c)
	snap := c.SetPlaces(nil)
	if snap.Len() != 1000 {
		t.Errorf("%d entities after places were set; want the change applied aside under them", snap.Len())
	}
	if again, _ := c.Flush(); again != snap {
		t.Error("the flush after set places returned another snapshot")
	}
}

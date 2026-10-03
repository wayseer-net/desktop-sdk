package data

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"wayseer.dev/sdk/model"
)

// ErrForeign reports a change to something the submitting module does not own.
var ErrForeign = errors.New("change to another module's data")

// CoalescerOptions configures a Coalescer; zero values take defaults.
type CoalescerOptions struct {
	Buffer     int                   // queued change sets before Submit waits; default 4096
	Publish    func(*model.Snapshot) // called once per commit, e.g. a kernel topic's Publish
	Now        func() time.Time      // clock for freshness; default time.Now
	StaleAfter time.Duration         // default 30 s
	Aside      int                   // a flush touching this many entities applies on its own goroutine; 0 never
}

// CoalescerStats counts traffic through a Coalescer.
type CoalescerStats struct {
	Submitted uint64 // change sets queued
	Rejected  uint64 // change sets refused as invalid, foreign or of a foreign kind
	Waits     uint64 // submits that found the queue full and had to wait
	Abandoned uint64 // waiting submits cancelled by their context
	Commits   uint64
}

// delta is one module's change set in the queue.
type delta struct {
	src     model.ModuleID
	cs      *model.ChangeSet
	replace bool // cs is the module's whole state
	remove  bool // src is gone: drop its state and its freshness
}

// Coalescer merges change sets from many module goroutines into one store commit per Flush.
type Coalescer struct {
	store   *model.Store
	in      chan delta
	publish func(*model.Snapshot)
	now     func() time.Time
	fresh   *Freshness
	batch   model.Batch             // owned by the flushing goroutine, or by the apply aside
	seen    map[model.ModuleID]bool // sources in the current flush: true sent, false removed
	queued  []delta                 // the current flush's change sets
	aside   int
	applied chan applied    // the apply aside's result; nil while none runs
	before  *model.Snapshot // the snapshot Flush returns until the apply aside lands
	late    error           // a commit's skipped items, for the next Flush to report
	kinds   confined
	stats   struct{ submitted, rejected, waits, abandoned, commits atomic.Uint64 }
}

// applied is a commit made aside from the flushing goroutine.
type applied struct {
	snap *model.Snapshot
	err  error
}

// NewCoalescer returns a Coalescer committing into store.
func NewCoalescer(store *model.Store, o CoalescerOptions) *Coalescer {
	if o.Buffer <= 0 {
		o.Buffer = 4096
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.StaleAfter <= 0 {
		o.StaleAfter = 30 * time.Second
	}
	if o.Publish == nil {
		o.Publish = func(*model.Snapshot) {}
	}
	return &Coalescer{
		store: store, in: make(chan delta, o.Buffer), publish: o.Publish, now: o.Now,
		fresh: NewFreshness(o.StaleAfter), seen: map[model.ModuleID]bool{}, aside: o.Aside,
	}
}

// Freshness returns the per-module freshness the coalescer maintains.
func (c *Coalescer) Freshness() *Freshness { return c.fresh }

// Submit queues cs from module src, waiting while the queue is full. The coalescer, and then
// the store, own cs and everything it refers to afterwards. An empty cs still marks src as seen.
func (c *Coalescer) Submit(ctx context.Context, src model.ModuleID, cs *model.ChangeSet) error {
	return c.enqueue(ctx, delta{src: src, cs: cs})
}

// SubmitSnapshot is Submit for src's whole state: whatever src owns and cs omits is removed.
func (c *Coalescer) SubmitSnapshot(ctx context.Context, src model.ModuleID, cs *model.ChangeSet) error {
	return c.enqueue(ctx, delta{src: src, cs: cs, replace: true})
}

// RemoveSource queues the removal of everything src owns and of its freshness, as one change
// ordered after what src has already sent.
func (c *Coalescer) RemoveSource(ctx context.Context, src model.ModuleID) error {
	c.kinds.free(src)
	return c.enqueue(ctx, delta{src: src, cs: &model.ChangeSet{}, replace: true, remove: true})
}

func (c *Coalescer) enqueue(ctx context.Context, d delta) error {
	if err := errors.Join(checkOwned(d.src, d.cs), c.kinds.check(d.src, d.cs)); err != nil {
		c.stats.rejected.Add(1)
		return err
	}
	select {
	case c.in <- d:
		c.stats.submitted.Add(1)
		return nil
	default:
	}
	c.stats.waits.Add(1)
	select {
	case c.in <- d:
		c.stats.submitted.Add(1)
		return nil
	case <-ctx.Done():
		c.stats.abandoned.Add(1)
		return ctx.Err()
	}
}

// Flush commits everything queued so far as one transaction and publishes the snapshot. It
// never waits for submitters. With nothing queued it returns the current snapshot. A commit
// touching Aside entities or more is applied on its own goroutine: Flush returns the snapshot
// before it, and the Flush after it finishes publishes and returns its result.
func (c *Coalescer) Flush() (*model.Snapshot, error) {
	snap, err := c.flush()
	err, c.late = errors.Join(c.late, err), nil
	return snap, err
}

func (c *Coalescer) flush() (*model.Snapshot, error) {
	if c.applied != nil {
		select {
		case r := <-c.applied:
			return c.landed(r)
		default:
			return c.before, nil
		}
	}
	n := len(c.in)
	if n == 0 {
		return c.store.Current(), nil
	}
	size := 0
	for range n {
		d := <-c.in
		c.queued = append(c.queued, d)
		c.seen[d.src] = !d.remove
		size += d.size(c.store.Current())
	}
	if c.aside > 0 && size >= c.aside {
		done := make(chan applied, 1)
		c.applied, c.before = done, c.store.Current()
		go func() { done <- c.apply() }()
		return c.before, nil
	}
	return c.landed(c.apply())
}

// Settle waits for a commit applied aside, then commits what is queued on this goroutine.
func (c *Coalescer) Settle() (*model.Snapshot, error) {
	var err error
	if c.applied != nil {
		_, err = c.landed(<-c.applied)
	}
	aside := c.aside
	c.aside = 0
	defer func() { c.aside = aside }()
	snap, err2 := c.Flush()
	return snap, errors.Join(err, err2)
}

// apply merges the queued change sets and commits them, if there is anything to commit.
func (c *Coalescer) apply() applied {
	for _, d := range c.queued {
		if d.replace {
			c.batch.Replace(d.src, c.store.Current(), d.cs)
		} else {
			c.batch.Add(d.cs)
		}
	}
	clear(c.queued)
	c.queued = c.queued[:0]
	cs := c.batch.ChangeSet()
	c.batch.Reset()
	if cs.Empty() {
		return applied{} // only heartbeats: nothing to commit
	}
	snap, err := c.store.Apply(&cs)
	return applied{snap: snap, err: err}
}

// landed records a commit's sources as seen and publishes its snapshot, if it made one.
func (c *Coalescer) landed(r applied) (*model.Snapshot, error) {
	c.applied, c.before = nil, nil
	c.markSeen()
	if r.snap == nil {
		return c.store.Current(), nil
	}
	c.stats.commits.Add(1)
	c.publish(r.snap)
	return r.snap, r.err
}

// size is about how many entities and edges d touches; a whole state counts what it replaces.
func (d delta) size(base *model.Snapshot) int {
	n := len(d.cs.Upserts) + len(d.cs.Removes) + len(d.cs.Edges) + len(d.cs.RemoveEdges)
	if d.replace {
		n += base.SourceLen(d.src)
	}
	return n
}

// SetPlaces swaps the places config names and places the world again, publishing the result;
// on the flushing goroutine only. It waits for a commit applied aside.
func (c *Coalescer) SetPlaces(g *model.Gazetteer) *model.Snapshot {
	if c.applied != nil {
		_, c.late = c.landed(<-c.applied)
	}
	snap := c.store.SetPlaces(g)
	c.stats.commits.Add(1)
	c.publish(snap)
	return snap
}

// markSeen records this flush's sources in freshness, forgetting those removed last.
func (c *Coalescer) markSeen() {
	var sent, gone []model.ModuleID
	for src, ok := range c.seen {
		if ok {
			sent = append(sent, src)
		} else {
			gone = append(gone, src)
		}
	}
	clear(c.seen)
	c.fresh.Seen(c.now(), sent...)
	c.fresh.Forget(gone...)
}

// Stats returns the traffic counters.
func (c *Coalescer) Stats() CoalescerStats {
	return CoalescerStats{
		Submitted: c.stats.submitted.Load(), Rejected: c.stats.rejected.Load(),
		Waits: c.stats.waits.Load(), Abandoned: c.stats.abandoned.Load(), Commits: c.stats.commits.Load(),
	}
}

// checkOwned validates cs and checks that everything it writes or removes belongs to src.
// Edges may point at other modules' entities, and edge removals are not checked.
func checkOwned(src model.ModuleID, cs *model.ChangeSet) error {
	var errs []error
	for _, r := range cs.Removes {
		inst, _, _, err := model.ParseEntityRef(string(r))
		errs = append(errs, err)
		if err == nil && model.ModuleID(inst) != src {
			errs = append(errs, fmt.Errorf("%w: %s removes %s", ErrForeign, src, r))
		}
	}
	for i := range cs.Upserts {
		e := &cs.Upserts[i]
		errs = append(errs, e.Validate())
		if e.Source != src {
			errs = append(errs, fmt.Errorf("%w: %s upserts %s", ErrForeign, src, e.Ref))
		}
	}
	for i := range cs.Edges {
		e := &cs.Edges[i]
		errs = append(errs, e.Validate())
		if e.Source != src {
			errs = append(errs, fmt.Errorf("%w: %s writes edge %v", ErrForeign, src, e.Key()))
		}
	}
	for i := range cs.Events {
		if e := &cs.Events[i]; e.Source != src {
			errs = append(errs, fmt.Errorf("%w: %s emits event %q", ErrForeign, src, e.ID))
		}
	}
	return errors.Join(errs...)
}

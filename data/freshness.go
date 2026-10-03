package data

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"wayseer.dev/sdk/model"
)

// State is how current a module's data is.
type State uint8

// Freshness states, in increasing severity.
const (
	FreshLive         State = iota // data arrived within the stale threshold
	FreshStale                     // healthy, but no data for longer than the threshold
	FreshDisconnected              // the module reports it cannot reach its source, or never started
	FreshError                     // the module reports an error
)

func (s State) String() string {
	return [...]string{"live", "stale", "disconnected", "error"}[s]
}

// Health is what a module reports about its connection to its source.
type Health struct {
	Disconnected bool
	Err          error
	Note         string // a limit worth showing that is not an error, e.g. an optional source missing
}

// moduleStatus is one module's freshness inputs.
type moduleStatus struct {
	lastSeen time.Time     // zero until the first data arrives
	gap      time.Duration // between the last two arrivals: the module's pace
	health   Health
}

// Freshness tracks each module's last data and health. Writers lock; readers load an
// immutable copy, so the render goroutine never waits.
type Freshness struct {
	staleAfter time.Duration
	mu         sync.Mutex
	view       atomic.Pointer[map[model.ModuleID]moduleStatus]
}

// maxGap is the longest pace counted, so an outage does not keep a module live for long after.
const maxGap = 5 * time.Minute

// NewFreshness returns a tracker that calls data stale after staleAfter, or after two of a
// slower module's gaps between arrivals.
func NewFreshness(staleAfter time.Duration) *Freshness {
	f := &Freshness{staleAfter: staleAfter}
	f.view.Store(&map[model.ModuleID]moduleStatus{})
	return f
}

// Seen records that data from each of ids arrived at t.
func (f *Freshness) Seen(t time.Time, ids ...model.ModuleID) {
	f.update(ids, func(s *moduleStatus) {
		if !s.lastSeen.IsZero() {
			s.gap = t.Sub(s.lastSeen)
		}
		s.lastSeen = t
	})
}

// SetHealth records id's latest health report.
func (f *Freshness) SetHealth(id model.ModuleID, h Health) {
	f.update([]model.ModuleID{id}, func(s *moduleStatus) { s.health = h })
}

// Forget drops ids, as if never heard of.
func (f *Freshness) Forget(ids ...model.ModuleID) {
	if len(ids) == 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	next := maps.Clone(*f.view.Load())
	for _, id := range ids {
		delete(next, id)
	}
	f.view.Store(&next)
}

// update publishes a copy of the view with fn applied to each of ids.
func (f *Freshness) update(ids []model.ModuleID, fn func(*moduleStatus)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	next := maps.Clone(*f.view.Load())
	for _, id := range ids {
		s := next[id]
		fn(&s)
		next[id] = s
	}
	f.view.Store(&next)
}

// Health returns id's latest health report.
func (f *Freshness) Health(id model.ModuleID) Health { return (*f.view.Load())[id].health }

// State classifies id at now: reported problems first, then the age of its data.
func (f *Freshness) State(id model.ModuleID, now time.Time) State {
	s, ok := (*f.view.Load())[id]
	switch {
	case !ok:
		return FreshDisconnected
	case s.health.Err != nil:
		return FreshError
	case s.health.Disconnected:
		return FreshDisconnected
	case s.lastSeen.IsZero() || now.Sub(s.lastSeen) > max(f.staleAfter, 2*min(s.gap, maxGap)):
		return FreshStale
	}
	return FreshLive
}

// Modules lists every module the tracker has heard of, sorted.
func (f *Freshness) Modules() []model.ModuleID {
	return slices.Sorted(maps.Keys(*f.view.Load()))
}

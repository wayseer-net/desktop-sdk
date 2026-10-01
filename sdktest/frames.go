package sdktest

import (
	"context"
	"sync"
	"wayseer/internal/data"
	"wayseer/internal/model"
	"wayseer/pkg/sdk"
)

// Frames is a Sink that feeds the app's coalescer, which commits what arrived since the last
// Flush as one change, as the app does once per frame.
type Frames struct {
	co  *data.Coalescer
	src sdk.ModuleID

	mu   sync.Mutex
	last *model.Snapshot
}

// NewFrames takes change sets from the module instance src.
func NewFrames(src sdk.ModuleID) *Frames {
	co := data.NewCoalescer(model.NewStore(model.StoreOptions{}), data.CoalescerOptions{})
	last, _ := co.Flush()
	return &Frames{co: co, src: src, last: last}
}

// Snapshot queues cs as the module's whole state.
func (f *Frames) Snapshot(ctx context.Context, cs *sdk.ChangeSet) error {
	return f.co.SubmitSnapshot(ctx, f.src, cs)
}

// Delta queues cs.
func (f *Frames) Delta(ctx context.Context, cs *sdk.ChangeSet) error {
	return f.co.Submit(ctx, f.src, cs)
}

// Flush commits everything queued, as one frame.
func (f *Frames) Flush() error {
	snap, err := f.co.Flush()
	f.mu.Lock()
	f.last = snap
	f.mu.Unlock()
	return err
}

// Entity returns r as the last Flush left it.
func (f *Frames) Entity(r sdk.EntityRef) (sdk.Entity, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.last.Entity(r); ok {
		return *e, true
	}
	return sdk.Entity{}, false
}

// Counts are how many change sets have been queued, and how many commits the flushes made.
func (f *Frames) Counts() (queued, commits uint64) {
	s := f.co.Stats()
	return s.Submitted, s.Commits
}

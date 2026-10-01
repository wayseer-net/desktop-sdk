package sdktest

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
	"wayseer/pkg/sdk"
)

// Sink records what a module sends, snapshot first.
type Sink struct {
	mu   sync.Mutex
	sets []sdk.ChangeSet
}

// Snapshot records cs.
func (s *Sink) Snapshot(_ context.Context, cs *sdk.ChangeSet) error { return s.add(cs) }

// Delta records cs.
func (s *Sink) Delta(_ context.Context, cs *sdk.ChangeSet) error { return s.add(cs) }

func (s *Sink) add(cs *sdk.ChangeSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets = append(s.sets, *cs)
	return nil
}

// Sets returns what was sent so far.
func (s *Sink) Sets() []sdk.ChangeSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.sets)
}

// Events returns every event sent so far, in order.
func (s *Sink) Events() []sdk.Event {
	var out []sdk.Event
	for _, cs := range s.Sets() {
		out = append(out, cs.Events...)
	}
	return out
}

// WaitFor waits until at least n change sets have been sent.
func (s *Sink) WaitFor(t *testing.T, n int) {
	t.Helper()
	Eventually(t, func() bool { return len(s.Sets()) >= n })
}

// Run runs m's Run with s in the background until the test ends.
func Run(t *testing.T, run func(context.Context, *Sink) error) *Sink {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	s, done := &Sink{}, make(chan error, 1)
	go func() { done <- run(ctx, s) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return s
}

// Eventually polls cond until it holds, failing the test after thirty seconds, or longer under
// the race detector; the wait is long so slow CI runners and emulators pass.
func Eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(slowdown * 30 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
	}
}

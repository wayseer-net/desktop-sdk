package sdktest

import (
	"context"
	"errors"
	"fmt"
	"time"
	"wayseer/internal/data"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/model"
)

// cancelLimit is how soon Run must return once its context is cancelled.
const cancelLimit = time.Second

// session is one live run of a working instance, feeding a real store.
type session struct {
	c      Case
	m      sdk.Module
	co     *data.Coalescer
	rec    *recorder
	cancel context.CancelFunc
	done   chan error
}

// runSession starts the module, runs the checks that need it live, then cancels it.
func runSession(c Case) []Result {
	names := []string{CheckSnapshot, CheckHealthy, CheckDiscover, CheckSeries, CheckEvents, CheckSearch, CheckActions, CheckCancel}
	m, err := configured(c, c.Options)
	if err != nil {
		return failAll(names, fmt.Errorf("Configure: %w", err))
	}
	s := start(c, m)
	if err := s.waitForSnapshot(); err != nil {
		_ = s.stop() // the missing snapshot is the finding
		return failAll(names, err)
	}
	time.Sleep(c.Observe)
	world, flushErr := s.co.Flush()
	return []Result{
		{CheckSnapshot, errors.Join(s.rec.violations(), flushErr)},
		{CheckHealthy, s.healthy()},
		{CheckDiscover, s.checkDiscover()},
		{CheckSeries, s.checkSeries(world)},
		{CheckEvents, s.checkEvents()},
		{CheckSearch, s.checkSearch(world)},
		{CheckActions, s.checkActions(world)},
		{CheckCancel, s.stop()},
	}
}

func failAll(names []string, err error) []Result {
	out := make([]Result, len(names))
	for i, n := range names {
		out[i] = Result{n, err}
	}
	return out
}

func start(c Case, m sdk.Module) *session {
	co := data.NewCoalescer(model.NewStore(model.StoreOptions{}), data.CoalescerOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		c: c, m: m, co: co, cancel: cancel, done: make(chan error, 1),
		rec: &recorder{co: co, name: c.Name, first: make(chan struct{})},
	}
	go func() { s.done <- m.Run(ctx, s.rec) }()
	return s
}

func (s *session) waitForSnapshot() error {
	select {
	case <-s.rec.first:
		return nil
	case err := <-s.done:
		s.done <- err // stop reads it again
		return fmt.Errorf("Run returned before sending anything: %v", err)
	case <-time.After(s.c.Timeout):
		return fmt.Errorf("no snapshot within %v", s.c.Timeout)
	}
}

func (s *session) healthy() error {
	if err := s.m.Health().Err; err != nil {
		return fmt.Errorf("Health reports an error with working options: %w", err)
	}
	return nil
}

// stop cancels Run and requires it to return promptly and cleanly.
func (s *session) stop() error {
	select {
	case err := <-s.done:
		s.cancel()
		return fmt.Errorf("%w: Run had already returned (%v)", ErrSkipped, err)
	default:
	}
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("Run returned %w on cancellation, want nil", err)
		}
		return nil
	case <-time.After(cancelLimit):
		return fmt.Errorf("Run still running %v after cancellation", cancelLimit)
	}
}

func (s *session) checkDiscover() error {
	d, ok := s.m.(sdk.Discoverer)
	if !ok {
		return fmt.Errorf("%w: not a Discoverer", ErrSkipped)
	}
	if err := s.withheld(func(ctx context.Context) error { _, err := d.Discover(ctx); return err }); err != nil {
		return err
	}
	return s.bounded(func(ctx context.Context) error {
		cs, err := d.Discover(ctx)
		if err != nil {
			return fmt.Errorf("Discover: %w", err)
		}
		co := data.NewCoalescer(model.NewStore(model.StoreOptions{}), data.CoalescerOptions{})
		if err := co.SubmitSnapshot(ctx, s.c.Name, cs); err != nil {
			return fmt.Errorf("Discover returned an unusable snapshot: %w", err)
		}
		_, err = co.Flush()
		return err
	})
}

// withheld is a skip when probe says the module does not offer the query, as an external
// module's host does for what its process lacks.
func (s *session) withheld(probe func(context.Context) error) error {
	if err := s.bounded(probe); errors.Is(err, sdk.ErrNotOffered) {
		return fmt.Errorf("%w: %v", ErrSkipped, err)
	}
	return nil
}

// bounded runs one query under the case timeout.
func (s *session) bounded(f func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.c.Timeout)
	defer cancel()
	return f(ctx)
}

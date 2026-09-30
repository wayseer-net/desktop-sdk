package sdktest

import (
	"context"
	"errors"
	"fmt"
	"mindseye/internal/data"
	"mindseye/pkg/sdk"
	"sync"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// Case describes a module under test.
type Case struct {
	New     func() sdk.Module
	Name    sdk.ModuleID  // instance name; default "conformance"
	Options string        // YAML options for a working instance
	Failing string        // YAML options that cannot reach the source; empty skips that check
	Timeout time.Duration // for the first snapshot and for errors to surface; default 5 s
	Observe time.Duration // how long to watch deltas after the snapshot; default 200 ms
}

// Check names, in the order they run.
const (
	CheckInfo         = "info"
	CheckStrict       = "strict-options"
	CheckSnapshot     = "snapshot-then-delta"
	CheckHealthy      = "healthy"
	CheckDiscover     = "discover"
	CheckSeries       = "series-queries"
	CheckEvents       = "event-queries"
	CheckSearch       = "search"
	CheckActions      = "actions"
	CheckCancel       = "cancellation"
	CheckHealthErrors = "health-errors"
)

// ErrSkipped marks a check that does not apply, such as a query the module does not support.
var ErrSkipped = errors.New("skipped")

// Result is one check's outcome; a nil Err passes.
type Result struct {
	Check string
	Err   error
}

// Conform runs the conformance suite against c as subtests of t.
func Conform(t *testing.T, c Case) {
	t.Helper()
	for _, r := range Check(c) {
		t.Run(r.Check, func(t *testing.T) {
			switch {
			case errors.Is(r.Err, ErrSkipped):
				t.Skip(r.Err)
			case r.Err != nil:
				t.Error(r.Err)
			}
		})
	}
}

// Check runs every check against c and returns the results in order.
func Check(c Case) []Result {
	c = c.withDefaults()
	out := []Result{
		{CheckInfo, checkInfo(c)},
		{CheckStrict, checkStrict(c)},
	}
	out = append(out, runSession(c)...)
	return append(out, Result{CheckHealthErrors, checkHealthErrors(c)})
}

func (c Case) withDefaults() Case {
	if c.Name == "" {
		c.Name = "conformance"
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.Observe <= 0 {
		c.Observe = 200 * time.Millisecond
	}
	return c
}

func checkInfo(c Case) error {
	i := c.New().Info()
	if err := sdk.ModuleID(i.Kind).Validate(); err != nil {
		return fmt.Errorf("kind: %w", err)
	}
	if i.Version == "" {
		return errors.New("empty version")
	}
	return nil
}

// checkStrict requires Configure to reject an unknown option.
func checkStrict(c Case) error {
	cfg, err := Config(c.Name, c.Options)
	if err != nil {
		return err
	}
	cfg.Options.Kind, cfg.Options.Tag = yaml.MappingNode, "!!map"
	cfg.Options.Content = append(cfg.Options.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "no_such_option", Line: cfg.Options.Line + 1},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "1", Line: cfg.Options.Line + 1})
	if c.New().Configure(context.Background(), cfg) == nil {
		return errors.New("Configure accepted an unknown option")
	}
	return nil
}

// checkHealthErrors requires an unreachable source to show as a Configure error, a Run error,
// or Health().Err within the timeout.
func checkHealthErrors(c Case) error {
	if c.Failing == "" {
		return fmt.Errorf("%w: no failing options given", ErrSkipped)
	}
	m, err := configured(c, c.Failing)
	if err != nil {
		return nil // surfaced at configuration
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, discardSink{}) }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return errors.New("Run hid the failure: it returned without an error")
		case <-tick.C:
			if m.Health().Err != nil {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("no error surfaced within %v", c.Timeout)
		}
	}
}

func configured(c Case, options string) (sdk.Module, error) {
	cfg, err := Config(c.Name, options)
	if err != nil {
		return nil, err
	}
	m := c.New()
	return m, m.Configure(context.Background(), cfg)
}

// Config is an instance named name with options parsed from YAML, as the config file gives them.
func Config(name sdk.ModuleID, options string) (sdk.Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(options), &doc); err != nil {
		return sdk.Config{}, fmt.Errorf("case options: %w", err)
	}
	cfg := sdk.Config{Name: name, Line: 1}
	if len(doc.Content) > 0 {
		cfg.Options = *doc.Content[0]
	}
	return cfg, nil
}

type discardSink struct{}

func (discardSink) Snapshot(context.Context, *sdk.ChangeSet) error { return nil }
func (discardSink) Delta(context.Context, *sdk.ChangeSet) error    { return nil }

// recorder is the Sink for the session: it submits through a real coalescer and notes violations.
type recorder struct {
	co    *data.Coalescer
	name  sdk.ModuleID
	first chan struct{}
	once  sync.Once
	mu    sync.Mutex
	calls int
	errs  []error
}

func (r *recorder) Snapshot(ctx context.Context, cs *sdk.ChangeSet) error {
	r.note()
	return r.check(r.co.SubmitSnapshot(ctx, r.name, cs))
}

func (r *recorder) Delta(ctx context.Context, cs *sdk.ChangeSet) error {
	if r.note() == 0 {
		return r.check(errors.New("the first change set was a Delta, not a Snapshot"))
	}
	return r.check(r.co.Submit(ctx, r.name, cs))
}

// note counts a call, returning how many came before it.
func (r *recorder) note() int {
	r.once.Do(func() { close(r.first) })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.calls - 1
}

func (r *recorder) check(err error) error {
	if err != nil {
		r.mu.Lock()
		r.errs = append(r.errs, err)
		r.mu.Unlock()
	}
	return err
}

func (r *recorder) violations() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.errs...)
}

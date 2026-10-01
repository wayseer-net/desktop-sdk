package sdktest_test

import (
	"context"
	"fmt"
	"time"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/sdktest"
)

// clock is a module with one entity, sent once and confirmed each second.
type clock struct{ name sdk.ModuleID }

func (*clock) Info() sdk.Info { return sdk.Info{Kind: "clock", Version: "1"} }

func (c *clock) Configure(_ context.Context, cfg sdk.Config) error {
	c.name = cfg.Name
	var none struct{}
	return cfg.Decode(&none)
}

func (c *clock) Run(ctx context.Context, sink sdk.Sink) error {
	ref, _ := sdk.NewEntityRef(string(c.name), sdk.KindHost, "here")
	cs := &sdk.ChangeSet{Upserts: []sdk.Entity{{Ref: ref, Kind: sdk.KindHost, Name: "here", Source: c.name, Seen: time.Now()}}}
	if err := sink.Snapshot(ctx, cs); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := sink.Delta(ctx, &sdk.ChangeSet{}); err != nil {
				return err
			}
		}
	}
}

func (*clock) Health() sdk.Health { return sdk.Health{} }

// In a module's tests, Conform(t, c) runs the same checks as subtests. Checks for queries the
// module does not answer, and for failure when no failing options are given, are skipped.
func ExampleCheck() {
	for _, r := range sdktest.Check(sdktest.Case{New: func() sdk.Module { return &clock{} }}) {
		fmt.Printf("%-20s %v\n", r.Check, r.Err)
	}
	// Output:
	// info                 <nil>
	// strict-options       <nil>
	// snapshot-then-delta  <nil>
	// healthy              <nil>
	// discover             skipped: not a Discoverer
	// series-queries       skipped: not a SeriesQuerier
	// event-queries        skipped: not an EventQuerier
	// search               skipped: not a Searcher
	// actions              skipped: not an Actor
	// cancellation         <nil>
	// health-errors        skipped: no failing options given
}

package sdktest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"wayseer/pkg/sdk"
)

// flaw is one way the fake module can break the contract.
type flaw int

const (
	sound flaw = iota
	deltaFirst
	foreignEntity
	ignoresCancel
	errorOnCancel
	lenientOptions
	hidesFailure
	seriesOutsideWindow
	searchOverLimit
	eventsOverLimit
	unknownUnit
	invalidAction
	actsWhenCancelled
	negativeTraffic
	placeOffEarth
)

type fakeOptions struct {
	Hosts       int  `yaml:"hosts"`
	Unreachable bool `yaml:"unreachable"`
}

// fake is a module with every capability; its flaw decides which check it fails.
type fake struct {
	flaw    flaw
	release <-chan struct{} // ends a Run that ignores cancellation
	opts    fakeOptions
}

func (f *fake) Info() sdk.Info { return sdk.Info{Kind: "fake", Version: "1"} }

func (f *fake) Configure(_ context.Context, c sdk.Config) error {
	if f.flaw == lenientOptions {
		return nil
	}
	return c.Decode(&f.opts)
}

func (f *fake) Run(ctx context.Context, s sdk.Sink) error {
	if f.opts.Unreachable && f.flaw != hidesFailure {
		return errors.New("connection refused")
	}
	first := s.Snapshot
	if f.flaw == deltaFirst {
		first = s.Delta
	}
	cs, _ := f.Discover(ctx)
	if err := first(ctx, cs); err != nil {
		return err
	}
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return f.finish(ctx)
		case <-time.After(5 * time.Millisecond):
			e := f.host(0)
			e.Attrs = map[string]sdk.Value{"cpu": sdk.Number(float64(i))}
			if err := s.Delta(ctx, &sdk.ChangeSet{Upserts: []sdk.Entity{e}}); err != nil {
				return err
			}
		}
	}
}

func (f *fake) finish(ctx context.Context) error {
	switch f.flaw {
	case ignoresCancel:
		<-f.release
	case errorOnCancel:
		return errors.New("stream closed")
	}
	return ctx.Err()
}

func (f *fake) Health() sdk.Health { return sdk.Health{} }

func (f *fake) host(i int) sdk.Entity {
	src := sdk.ModuleID("conformance")
	if f.flaw == foreignEntity && i == 0 {
		src = "someone-else"
	}
	name := fmt.Sprintf("h%d", i)
	r, _ := sdk.NewEntityRef(string(src), sdk.KindHost, name)
	place := sdk.At(52.37, 4.9)
	if f.flaw == placeOffEarth {
		place.Lat = 152.37
	}
	return sdk.Entity{Ref: r, Kind: sdk.KindHost, Name: name, Source: src, Place: place}
}

func (f *fake) Discover(ctx context.Context) (*sdk.ChangeSet, error) {
	var cs sdk.ChangeSet
	for i := range f.opts.Hosts {
		cs.Upserts = append(cs.Upserts, f.host(i))
	}
	if len(cs.Upserts) >= 2 {
		rate := 20.0
		if f.flaw == negativeTraffic {
			rate = -20
		}
		cs.Edges = []sdk.Edge{{
			From: cs.Upserts[0].Ref, To: cs.Upserts[1].Ref, Rel: sdk.RelTalksTo, Source: "conformance",
			Traffic: sdk.Traffic{Rate: rate, Unit: sdk.TrafficRequests},
		}}
	}
	return &cs, ctx.Err()
}

func (f *fake) Metrics() []sdk.Metric {
	unit := sdk.UnitPercent
	if f.flaw == unknownUnit {
		unit = "percentage"
	}
	return []sdk.Metric{{Name: "cpu", Unit: unit, Kinds: []sdk.Kind{sdk.KindHost}}}
}

func (f *fake) QuerySeries(ctx context.Context, q sdk.SeriesQuery) ([]sdk.Series, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []sdk.Series
	for _, r := range q.Entities {
		sr := sdk.Series{Ref: sdk.SeriesRef{Entity: r, Metric: q.Metrics[0]}}
		for t := q.Window.From; t.Before(q.Window.To); t = t.Add(q.Step) {
			sr.Points = append(sr.Points, sdk.Point{T: t.UnixNano(), V: 1})
		}
		if f.flaw == seriesOutsideWindow {
			sr.Points = append(sr.Points, sdk.Point{T: q.Window.To.UnixNano()})
		}
		out = append(out, sr)
	}
	return out, nil
}

func (f *fake) QueryEvents(ctx context.Context, q sdk.EventQuery) ([]sdk.Event, error) {
	n := 2
	if f.flaw == eventsOverLimit {
		n = q.Limit + 1
	}
	out := make([]sdk.Event, n)
	for i := range out {
		out[i] = sdk.Event{ID: fmt.Sprint(i), At: q.Window.To.Add(-time.Second), Source: "conformance"}
	}
	return out, ctx.Err()
}

func (f *fake) Search(ctx context.Context, _ string, limit int) ([]sdk.EntityRef, error) {
	n := 1
	if f.flaw == searchOverLimit {
		n = limit + 1
	}
	out := make([]sdk.EntityRef, n)
	for i := range out {
		out[i] = f.host(i + 1).Ref
	}
	return out, ctx.Err()
}

func (f *fake) Actions() []sdk.Action {
	a := sdk.Action{
		ID: "scale", Title: "Scale", Changes: "Sets how many run",
		Kinds: []sdk.Kind{sdk.KindHost}, Params: []sdk.Param{sdk.IntParam("n", "Count", 1, 3)},
	}
	if f.flaw == invalidAction {
		a.Kinds = nil
	}
	return []sdk.Action{a}
}

func (f *fake) Do(ctx context.Context, _ sdk.ActionRequest) (sdk.ActionResult, error) {
	if f.flaw == actsWhenCancelled {
		return sdk.ActionResult{Message: "scaled"}, nil
	}
	return sdk.ActionResult{}, ctx.Err()
}

func fakeCase(t *testing.T, fl flaw) Case {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return Case{
		New:     func() sdk.Module { return &fake{flaw: fl, release: release} },
		Options: "hosts: 4",
		Failing: "hosts: 4\nunreachable: true",
		Timeout: 300 * time.Millisecond,
		Observe: 30 * time.Millisecond,
	}
}

func TestSoundModulePasses(t *testing.T) {
	Conform(t, fakeCase(t, sound))
}

func TestEachFlawFailsItsCheck(t *testing.T) {
	cases := map[flaw][]string{
		deltaFirst:          {CheckSnapshot},
		foreignEntity:       {CheckSnapshot, CheckDiscover},
		ignoresCancel:       {CheckCancel},
		errorOnCancel:       {CheckCancel},
		lenientOptions:      {CheckStrict, CheckHealthErrors}, // never reads unreachable
		hidesFailure:        {CheckHealthErrors},
		seriesOutsideWindow: {CheckSeries},
		searchOverLimit:     {CheckSearch},
		eventsOverLimit:     {CheckEvents},
		unknownUnit:         {CheckSeries},
		invalidAction:       {CheckActions},
		actsWhenCancelled:   {CheckActions},
		negativeTraffic:     {CheckSnapshot, CheckDiscover},
		placeOffEarth:       {CheckSnapshot, CheckDiscover},
	}
	for fl, want := range cases {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			t.Parallel()
			var failed []string
			for _, r := range Check(fakeCase(t, fl)) {
				if r.Err != nil && !errors.Is(r.Err, ErrSkipped) {
					failed = append(failed, r.Check)
				}
			}
			if strings.Join(failed, ",") != strings.Join(want, ",") {
				t.Errorf("failed %v, want %v", failed, want)
			}
		})
	}
}

// withholder has every query method but offers none, as an external module can.
type withholder struct{ *fake }

func (withholder) Discover(context.Context) (*sdk.ChangeSet, error) {
	return nil, sdk.NotOffered("discover")
}
func (withholder) Metrics() []sdk.Metric { return nil }
func (withholder) QuerySeries(context.Context, sdk.SeriesQuery) ([]sdk.Series, error) {
	return nil, sdk.NotOffered("series queries")
}

func (withholder) QueryEvents(context.Context, sdk.EventQuery) ([]sdk.Event, error) {
	return nil, sdk.NotOffered("event queries")
}

func (withholder) Search(context.Context, string, int) ([]sdk.EntityRef, error) {
	return nil, sdk.NotOffered("search")
}

func TestQueriesNotOfferedAreSkipped(t *testing.T) {
	c := fakeCase(t, sound)
	c.New = func() sdk.Module { return withholder{&fake{}} }
	for _, r := range Check(c) {
		switch r.Check {
		case CheckDiscover, CheckSeries, CheckEvents, CheckSearch:
			if !errors.Is(r.Err, ErrSkipped) {
				t.Errorf("%s: %v, want skipped", r.Check, r.Err)
			}
		default:
			if r.Err != nil {
				t.Errorf("%s: %v", r.Check, r.Err)
			}
		}
	}
}

package sdktest

import (
	"context"
	"errors"
	"fmt"
	"mindseye/internal/model"
	"mindseye/pkg/sdk"
	"slices"
	"time"
)

// queryWindow is the recent window the query checks ask about.
const queryWindow = 10 * time.Minute

// checkSeries queries every catalogue metric for a few entities and checks the answer's shape.
func (s *session) checkSeries(world *model.Snapshot) error {
	q, ok := s.m.(sdk.SeriesQuerier)
	if !ok {
		return fmt.Errorf("%w: not a SeriesQuerier", ErrSkipped)
	}
	if err := s.withheld(func(ctx context.Context) error { _, err := q.QuerySeries(ctx, sdk.SeriesQuery{}); return err }); err != nil {
		return err
	}
	metrics := q.Metrics()
	if err := checkCatalogue(metrics); err != nil {
		return err
	}
	var errs []error
	for _, m := range metrics {
		sq := seriesQuery(world, s.c.Name, m)
		errs = append(errs, s.bounded(func(ctx context.Context) error {
			got, err := q.QuerySeries(ctx, sq)
			if err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
			return checkSeriesShape(sq, got)
		}))
	}
	errs = append(errs, cancelledQuery(func(ctx context.Context) error {
		_, err := q.QuerySeries(ctx, seriesQuery(world, s.c.Name, metrics[0]))
		return err
	}))
	return errors.Join(errs...)
}

func checkCatalogue(metrics []sdk.Metric) error {
	if len(metrics) == 0 {
		return errors.New("empty metric catalogue")
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		if m.Name == "" || seen[m.Name] {
			return fmt.Errorf("catalogue: empty or duplicate metric name %q", m.Name)
		}
		seen[m.Name] = true
		if err := m.Unit.Validate(); err != nil {
			return fmt.Errorf("catalogue: metric %s: %w", m.Name, err)
		}
	}
	return nil
}

// seriesQuery asks for m on up to three of the module's entities that can have it.
func seriesQuery(world *model.Snapshot, src sdk.ModuleID, m sdk.Metric) sdk.SeriesQuery {
	var refs []sdk.EntityRef
	for r := range world.BySource(src) {
		if len(m.Kinds) == 0 || slices.Contains(m.Kinds, r.Kind()) {
			refs = append(refs, r)
		}
		if len(refs) == 3 {
			break
		}
	}
	now := time.Now()
	w := sdk.TimeWindow{From: now.Add(-queryWindow), To: now}
	return sdk.SeriesQuery{Entities: refs, Metrics: []string{m.Name}, Window: w, Step: sdk.StepFor(w, 100)}
}

// checkSeriesShape requires answers only for what was asked (no entities means the filter chose), with ordered points in the window.
func checkSeriesShape(q sdk.SeriesQuery, got []sdk.Series) error {
	for _, sr := range got {
		switch {
		case len(q.Entities) > 0 && !slices.Contains(q.Entities, sr.Ref.Entity):
			return fmt.Errorf("series for %s, which was not asked for", sr.Ref.Entity)
		case !slices.Contains(q.Metrics, sr.Ref.Metric):
			return fmt.Errorf("series of metric %q, which was not asked for", sr.Ref.Metric)
		}
		for i, p := range sr.Points {
			if !q.Window.Contains(p.T) {
				return fmt.Errorf("%v: point at %v outside the window", sr.Ref, p.Time())
			}
			if i > 0 && p.T <= sr.Points[i-1].T {
				return fmt.Errorf("%v: points out of order at %d", sr.Ref, i)
			}
		}
	}
	return nil
}

// checkEvents asks for a few recent events and checks they are the module's and in range.
func (s *session) checkEvents() error {
	q, ok := s.m.(sdk.EventQuerier)
	if !ok {
		return fmt.Errorf("%w: not an EventQuerier", ErrSkipped)
	}
	now := time.Now()
	eq := sdk.EventQuery{Window: sdk.TimeWindow{From: now.Add(-queryWindow), To: now.Add(time.Second)}, Limit: 5}
	if err := s.withheld(func(ctx context.Context) error { _, err := q.QueryEvents(ctx, eq); return err }); err != nil {
		return err
	}
	return errors.Join(
		s.bounded(func(ctx context.Context) error {
			got, err := q.QueryEvents(ctx, eq)
			if err != nil {
				return fmt.Errorf("QueryEvents: %w", err)
			}
			return checkEventShape(eq, s.c.Name, got)
		}),
		cancelledQuery(func(ctx context.Context) error { _, err := q.QueryEvents(ctx, eq); return err }),
	)
}

func checkEventShape(q sdk.EventQuery, src sdk.ModuleID, got []sdk.Event) error {
	if len(got) > q.Limit {
		return fmt.Errorf("%d events, limit %d", len(got), q.Limit)
	}
	for _, e := range got {
		switch {
		case e.Source != src:
			return fmt.Errorf("event %q has source %q", e.ID, e.Source)
		case !q.Window.Contains(e.At.UnixNano()):
			return fmt.Errorf("event %q at %v outside the window", e.ID, e.At)
		}
	}
	return nil
}

// checkSearch searches for an entity by name and checks the answer's shape.
func (s *session) checkSearch(world *model.Snapshot) error {
	q, ok := s.m.(sdk.Searcher)
	if !ok {
		return fmt.Errorf("%w: not a Searcher", ErrSkipped)
	}
	text := ""
	for r := range world.BySource(s.c.Name) {
		if e, _ := world.Entity(r); e.Name != "" {
			text = e.Name
			break
		}
	}
	const limit = 3
	if err := s.withheld(func(ctx context.Context) error { _, err := q.Search(ctx, text, limit); return err }); err != nil {
		return err
	}
	return errors.Join(
		s.bounded(func(ctx context.Context) error {
			got, err := q.Search(ctx, text, limit)
			if err != nil {
				return fmt.Errorf("Search: %w", err)
			}
			return checkSearchShape(s.c.Name, limit, got)
		}),
		cancelledQuery(func(ctx context.Context) error { _, err := q.Search(ctx, text, limit); return err }),
	)
}

func checkSearchShape(src sdk.ModuleID, limit int, got []sdk.EntityRef) error {
	if len(got) > limit {
		return fmt.Errorf("%d results, limit %d", len(got), limit)
	}
	for _, r := range got {
		if r.Validate() != nil || sdk.ModuleID(r.Instance()) != src {
			return fmt.Errorf("result %q is not one of the module's refs", r)
		}
	}
	return nil
}

// cancelledQuery requires a query under an already-cancelled context to fail promptly.
func cancelledQuery(query func(context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- query(ctx) }()
	select {
	case err := <-done:
		if err == nil {
			return errors.New("a query with a cancelled context succeeded")
		}
		return nil
	case <-time.After(cancelLimit):
		return fmt.Errorf("a query with a cancelled context ran for over %v", cancelLimit)
	}
}

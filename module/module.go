package module

import (
	"context"
	"log/slog"

	"wayseer.dev/sdk/data"
	"wayseer.dev/sdk/model"
)

// Module is a Tier 1 data source (PLAN §6.1). The host calls Configure once, then Run, possibly
// again after a failure; Health may be called at any time from another goroutine.
type Module interface {
	Info() Info
	// Configure decodes and checks options; it must not do I/O that can hang.
	Configure(ctx context.Context, cfg Config) error
	// Run sends a snapshot, then deltas, until ctx is cancelled; it must return within 1 s of
	// cancellation. An error or panic makes the host restart it with back-off.
	Run(ctx context.Context, sink Sink) error
	Health() data.Health
}

// Info describes a module kind.
type Info struct {
	Kind        string
	Version     string
	Description string
}

// Sink receives a module's world-model changes; the core batches them. The first call from
// each Run must be Snapshot. Both wait while the queue is full, until ctx ends. A sent change
// set, with its maps and slices, belongs to the core: the module must not modify it again.
type Sink interface {
	// Snapshot replaces everything the module owns with cs.
	Snapshot(ctx context.Context, cs *model.ChangeSet) error
	// Delta applies cs on top of what the module sent before. An empty cs says the source was
	// read and nothing changed; send one each successful read so the data does not go stale.
	Delta(ctx context.Context, cs *model.ChangeSet) error
}

// Discoverer lists the module's whole state on demand, as a Snapshot would send it.
type Discoverer interface {
	Discover(ctx context.Context) (*model.ChangeSet, error)
}

// SeriesQuerier answers series queries over the metrics in its catalogue.
type SeriesQuerier interface {
	Metrics() []Metric
	QuerySeries(ctx context.Context, q data.SeriesQuery) ([]data.Series, error)
}

// EventQuerier answers queries over past events.
type EventQuerier interface {
	QueryEvents(ctx context.Context, q EventQuery) ([]model.Event, error)
}

// Subscriber streams series matching q to fn until ctx is cancelled.
type Subscriber interface {
	Subscribe(ctx context.Context, q data.SeriesQuery, fn func([]data.Series)) error
}

// TopRanker is a SeriesQuerier that honours SeriesQuery.Top when RanksTop is true, so the
// app asks it once for the top entities, not for all of them in batches.
type TopRanker interface {
	RanksTop() bool
}

// Searcher finds the module's entities by free text, best matches first.
type Searcher interface {
	Search(ctx context.Context, text string, limit int) ([]model.EntityRef, error)
}

// Metric is one catalogue entry: a canonical metric and how the module computes it.
type Metric struct {
	Name        string // canonical, e.g. "cpu.utilisation"
	Unit        model.Unit
	Description string
	Kinds       []model.Kind // entity kinds that have it; empty means any
	Native      string       // the module's own expression, shown to power users
	Extra       bool         // one of many the source offers: chosen by name, not opened with its entity
	Joined      bool         // for other modules' entities of Kinds, which it names from its own data
}

// EventQuery selects past events; zero fields do not filter.
type EventQuery struct {
	Entities    []model.EntityRef
	Window      data.TimeWindow
	MinSeverity model.Severity
	Kinds       []string
	Limit       int // most recent first when set
}

// Capabilities names the optional interfaces m implements, for display.
func Capabilities(m Module) []string {
	var out []string
	if _, ok := m.(Discoverer); ok {
		out = append(out, "discover")
	}
	if _, ok := m.(SeriesQuerier); ok {
		out = append(out, "series")
	}
	if _, ok := m.(EventQuerier); ok {
		out = append(out, "events")
	}
	if _, ok := m.(Subscriber); ok {
		out = append(out, "subscribe")
	}
	if _, ok := m.(Searcher); ok {
		out = append(out, "search")
	}
	if _, ok := m.(Actor); ok {
		out = append(out, "actions")
	}
	if r, ok := m.(TopRanker); ok && r.RanksTop() {
		out = append(out, "top")
	}
	return out
}

// Confiner is a Sink that limits its run's entity kinds to the core kinds and a namespace. An
// external module confines itself before it sends, naming the namespace's owner, its signer.
type Confiner interface {
	Confine(namespace, owner string) error
}

// LogUser is a module that logs, such as one whose process writes to stderr.
type LogUser interface{ UseLog(*slog.Logger) }

// Resolver finds the entity a value names in the world; *model.Matcher is one.
type Resolver interface {
	Match(kind model.Kind, value string) (model.EntityRef, error)
	// MatchExcept is Match leaving out the entities skip names.
	MatchExcept(kind model.Kind, value string, skip func(model.EntityRef) bool) (model.EntityRef, error)
}

// WorldUser is a module that links what it reads to what other modules found. Each call to
// world matches against the world as it is then.
type WorldUser interface{ UseWorld(world func() Resolver) }

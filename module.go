package sdk

import (
	"mindseye/internal/data"
	"mindseye/internal/module"
)

// The module contract; see the internal package for each method's rules.
type (
	// Module is a data source the app configures once and runs until it stops.
	Module = module.Module
	// Info describes a module kind.
	Info = module.Info
	// Config is one instance's name, config line and options; Decode reads the options.
	Config = module.Config
	// Sink receives a module's snapshot, then its deltas.
	Sink = module.Sink
	// Factory makes an unconfigured module.
	Factory = module.Factory

	// Discoverer lists the module's whole state on demand.
	Discoverer = module.Discoverer
	// SeriesQuerier answers series queries over its metric catalogue.
	SeriesQuerier = module.SeriesQuerier
	// EventQuerier answers queries over past events.
	EventQuerier = module.EventQuerier
	// Subscriber streams series as they change.
	Subscriber = module.Subscriber
	// Searcher finds the module's entities by free text.
	Searcher = module.Searcher
	// TopRanker is a SeriesQuerier that answers SeriesQuery.Top with only the top entities.
	TopRanker = module.TopRanker

	// Metric is one catalogue entry.
	Metric = module.Metric
	// EventQuery selects past events.
	EventQuery = module.EventQuery

	// Tracker remembers what was sent, so each poll sends only the difference.
	Tracker = module.Tracker
	// EventLog keeps recent events to answer EventQuery.
	EventLog = module.EventLog

	// Health is what a module reports about its source.
	Health = data.Health
	// State is a module's freshness as the app shows it.
	State = data.State
)

// Freshness states.
const (
	FreshLive         = data.FreshLive
	FreshStale        = data.FreshStale
	FreshDisconnected = data.FreshDisconnected
	FreshError        = data.FreshError
)

// ErrNotOffered is what a query answers when the module does not offer it; callers skip it.
var ErrNotOffered = module.ErrNotOffered

// NotOffered is the error for a module that does not offer what, such as "event queries".
func NotOffered(what string) error { return module.NotOffered(what) }

// Register adds a module kind to the app; call it from the module package's init.
func Register(kind string, f Factory) { module.Register(kind, f) }

// NewEventLog keeps at most n events.
func NewEventLog(n int) *EventLog { return module.NewEventLog(n) }

// CompareEdgeKeys orders edge keys by from, to, then relation.
func CompareEdgeKeys(a, b EdgeKey) int { return module.CompareEdgeKeys(a, b) }

// Capabilities names the optional interfaces m implements.
func Capabilities(m Module) []string { return module.Capabilities(m) }

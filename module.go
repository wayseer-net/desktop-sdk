package sdk

import (
	"mindseye/internal/data"
	"mindseye/internal/module"
	"time"
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
	// WorldUser is a module that links what it reads to what other modules found.
	WorldUser = module.WorldUser
	// Resolver finds the entity a value names in the world, by name or the identity rules.
	Resolver = module.Resolver

	// TopRanker is a SeriesQuerier that answers SeriesQuery.Top with only the top entities.
	TopRanker = module.TopRanker

	// Actor is a module whose entities offer actions the owner may run.
	Actor = module.Actor
	// Action is one thing a module can do to an entity, such as restarting it.
	Action = module.Action
	// Param is one typed, bounded parameter of an action.
	Param = module.Param
	// ParamType is how a parameter's value is read.
	ParamType = module.ParamType
	// ActionRequest asks an instance to run an action on one of its entities.
	ActionRequest = module.ActionRequest
	// ActionResult is what an action did, in one line for the owner.
	ActionResult = module.ActionResult

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

// Parameter types.
const (
	ParamInt      = module.ParamInt
	ParamDuration = module.ParamDuration
	ParamChoice   = module.ParamChoice
)

// IntParam is a whole number from lo to hi.
func IntParam(name, title string, lo, hi int64) Param { return module.IntParam(name, title, lo, hi) }

// DurationParam is a duration from lo to hi.
func DurationParam(name, title string, lo, hi time.Duration) Param {
	return module.DurationParam(name, title, lo, hi)
}

// ChoiceParam is one of choices.
func ChoiceParam(name, title string, choices ...string) Param {
	return module.ChoiceParam(name, title, choices...)
}

// ValidateActions checks an Actor's catalogue as the app does when it loads the config.
func ValidateActions(acts []Action) error { return module.ValidateActions(acts) }

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

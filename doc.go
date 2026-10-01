// Package sdk is everything a Wayseer module imports: the module contract, the world-model
// types a module sends, series and events, option decoding and secrets.
//
// A module registers a kind at init. The app makes one instance per config entry of that
// kind, calls Configure with its options, then Run, which sends a snapshot and then deltas
// to a Sink until its context ends. Optional interfaces (Discoverer, SeriesQuerier,
// EventQuerier, Subscriber, Searcher) add what the module can answer on demand.
//
// Module errors reach logs and the screen, so they must never contain secrets.
package sdk

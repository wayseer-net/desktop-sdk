package model

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"
)

// ModuleID names a configured module instance.
type ModuleID string

// StatusLevel is an entity's health, ordered from least to most severe.
type StatusLevel uint8

// Status levels.
const (
	StatusUnknown StatusLevel = iota
	StatusOK
	StatusWarn
	StatusCrit
	StatusDown
)

var statusNames = [...]string{"unknown", "ok", "warn", "crit", "down"}

func (s StatusLevel) String() string {
	if int(s) < len(statusNames) {
		return statusNames[s]
	}
	return "status(" + strconv.Itoa(int(s)) + ")"
}

// Worse reports whether s is more severe than t.
func (s StatusLevel) Worse(t StatusLevel) bool { return s > t }

// Status is a health level with the reason for it.
type Status struct {
	Level  StatusLevel
	Reason string
}

// Severity grades events.
type Severity uint8

// Severities, least to most severe.
const (
	SevDebug Severity = iota
	SevInfo
	SevWarn
	SevError
	SevCritical
)

var severityNames = [...]string{"debug", "info", "warn", "error", "critical"}

func (s Severity) String() string {
	if int(s) < len(severityNames) {
		return severityNames[s]
	}
	return "severity(" + strconv.Itoa(int(s)) + ")"
}

// Unit describes what a series or numeric attribute measures.
type Unit string

// Units a module may declare; a ratio runs from 0 to 1 and shows as a percentage.
const (
	UnitNone    Unit = ""
	UnitBytes   Unit = "bytes"
	UnitBytesPS Unit = "bytes_per_second"
	UnitBits    Unit = "bits"
	UnitBitsPS  Unit = "bits_per_second"
	UnitPercent Unit = "percent"
	UnitRatio   Unit = "ratio"
	UnitSeconds Unit = "seconds"
	UnitCount   Unit = "count"
	UnitPerSec  Unit = "per_second"

	// Physical quantities, since contract 1.7; an older app shows them as plain numbers.
	UnitCelsius         Unit = "celsius"
	UnitWatts           Unit = "watts"
	UnitWattHours       Unit = "watt_hours"
	UnitVolts           Unit = "volts"
	UnitAmperes         Unit = "amperes"
	UnitHertz           Unit = "hertz"
	UnitLux             Unit = "lux"
	UnitPascals         Unit = "pascals"
	UnitPPM             Unit = "parts_per_million"
	UnitMicrogramsPerM3 Unit = "micrograms_per_cubic_metre"
	UnitDBm             Unit = "decibel_milliwatts"
)

var knownUnits = []Unit{
	UnitNone, UnitBytes, UnitBytesPS, UnitBits, UnitBitsPS, UnitPercent, UnitRatio, UnitSeconds, UnitCount, UnitPerSec,
	UnitCelsius, UnitWatts, UnitWattHours, UnitVolts, UnitAmperes, UnitHertz, UnitLux, UnitPascals, UnitPPM,
	UnitMicrogramsPerM3, UnitDBm,
}

// Validate checks that u is one of the known units.
func (u Unit) Validate() error {
	if slices.Contains(knownUnits, u) {
		return nil
	}
	return fmt.Errorf("unknown unit %q (want one of %q)", string(u), knownUnits[1:])
}

// Entity is a thing in the world: a host, a service, a table.
type Entity struct {
	Ref    EntityRef
	Kind   Kind
	Name   string
	Attrs  map[string]Value
	Status Status
	Tags   []string
	Source ModuleID
	Seen   time.Time // last time the source confirmed the entity exists
	Place  Place
}

// Place is where an entity is on Earth, in degrees; the zero value means none is known.
type Place struct {
	Lat, Lon float32 // float32 is within a metre, and keeps each entity small
	Known    bool
	From     PlaceSource // set by the store; a module's own place is FromModule
}

func (p Place) point() [2]float32 { return [2]float32{p.Lat, p.Lon} }

// PlaceSource says where the store found an entity's place.
type PlaceSource uint8

// Where a place came from, first to last in precedence.
const (
	FromModule     PlaceSource = iota // the module sent it
	FromConfig                        // an attribute names it in config's places
	FromMembership                    // the entity is a member of a placed entity
)

// At is the known place at a latitude and longitude.
func At(lat, lon float32) Place { return Place{Lat: lat, Lon: lon, Known: true} }

// Validate checks that a known place is on Earth, and an unknown one is zero.
func (p Place) Validate() error {
	switch {
	case !p.Known && p != Place{}:
		return fmt.Errorf("%w place: %v, %v is not marked known", errInvalid, p.Lat, p.Lon)
	case p.From > FromMembership:
		return fmt.Errorf("%w place: unknown source %d", errInvalid, p.From)
	case p.Known && !(p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180):
		return fmt.Errorf("%w place: %v, %v is not on Earth", errInvalid, p.Lat, p.Lon)
	}
	return nil
}

var errInvalid = errors.New("invalid")

// Validate checks that the ref is canonical and agrees with Kind and Source, and the place.
func (e *Entity) Validate() error {
	inst, kind, _, err := ParseEntityRef(string(e.Ref))
	switch {
	case err != nil:
		return err
	case kind != e.Kind:
		return fmt.Errorf("%w entity %s: kind %q differs from ref", errInvalid, e.Ref, e.Kind)
	case ModuleID(inst) != e.Source:
		return fmt.Errorf("%w entity %s: source %q differs from ref", errInvalid, e.Ref, e.Source)
	}
	if err := e.Place.Validate(); err != nil {
		return fmt.Errorf("entity %s: %w", e.Ref, err)
	}
	return nil
}

// Edge is a directed relation between two entities, possibly owned by different modules.
type Edge struct {
	From, To EntityRef
	Rel      Relation
	Weight   float64
	Attrs    map[string]Value
	Source   ModuleID
	Traffic  Traffic
}

// TrafficUnit is what an edge's traffic counts, in one byte since every edge has one.
type TrafficUnit uint8

// Traffic units; none means no traffic is known.
const (
	TrafficNone TrafficUnit = iota
	TrafficRequests
	TrafficBytes
	TrafficMessages
	trafficUnits // the count, and the first unknown unit
)

var trafficUnitNames = [trafficUnits]string{"", "requests", "bytes", "messages"}

func (u TrafficUnit) String() string {
	if u < trafficUnits {
		return trafficUnitNames[u]
	}
	return "unit(" + strconv.Itoa(int(u)) + ")"
}

// ParseTrafficUnit reads a unit's name, as in config or on the wire.
func ParseTrafficUnit(s string) (TrafficUnit, error) {
	if i := slices.Index(trafficUnitNames[:], s); i > 0 {
		return TrafficUnit(i), nil
	}
	return TrafficNone, fmt.Errorf("unknown traffic unit %q (want one of %q)", s, trafficUnitNames[1:])
}

// MarshalText writes the unit's name.
func (u TrafficUnit) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// UnmarshalText reads a unit's name.
func (u *TrafficUnit) UnmarshalText(b []byte) (err error) {
	*u, err = ParseTrafficUnit(string(b))
	return err
}

// Traffic is a rate per second along an edge; the zero value means none is known.
type Traffic struct {
	Rate float64
	Unit TrafficUnit
}

// Known reports whether the edge carries a rate.
func (t Traffic) Known() bool { return t.Unit != TrafficNone }

// Validate checks that a known rate is finite and not negative, in a known unit.
func (t Traffic) Validate() error {
	switch {
	case t == Traffic{}:
		return nil
	case t.Unit == TrafficNone || t.Unit >= trafficUnits:
		return fmt.Errorf("%w traffic: unit %v (want one of %q)", errInvalid, t.Unit, trafficUnitNames[1:])
	case !(t.Rate >= 0) || math.IsInf(t.Rate, 1):
		return fmt.Errorf("%w traffic: rate %v is not a rate", errInvalid, t.Rate)
	}
	return nil
}

// Validate checks both refs, the relation and the traffic; self edges are rejected.
func (e Edge) Validate() error {
	if err := errors.Join(e.From.Validate(), e.To.Validate(), e.Rel.Validate(), e.Traffic.Validate()); err != nil {
		return err
	}
	if e.From == e.To {
		return fmt.Errorf("%w edge: %s relates to itself", errInvalid, e.From)
	}
	return nil
}

// Event is something that happened, optionally to an entity.
type Event struct {
	ID       string
	Entity   EntityRef // empty for global events
	At       time.Time
	Severity Severity
	Kind     string // log, alert, deploy, restart, audit, ...
	Message  string
	Fields   map[string]Value
	Source   ModuleID
}

// Validate reports whether m is a usable instance name: lowercase, digits, '-', '_', '.'.
func (m ModuleID) Validate() error { return validInstance(string(m)) }

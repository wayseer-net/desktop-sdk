package sdk

import (
	"mindseye/internal/model"
	"time"
)

// The world-model types a module sends.
type (
	// ChangeSet is one snapshot or delta: upserts, removals, edges and events.
	ChangeSet = model.ChangeSet
	// Entity is one thing in the world, owned by the module that sends it.
	Entity = model.Entity
	// EntityRef names an entity: instance, kind and the source's own ID.
	EntityRef = model.EntityRef
	// ModuleID is a module instance's name.
	ModuleID = model.ModuleID
	// Kind is an entity's kind.
	Kind = model.Kind
	// Edge relates two entities.
	Edge = model.Edge
	// Traffic is a rate per second along an edge, in a unit.
	Traffic = model.Traffic
	// TrafficUnit is what an edge's traffic counts.
	TrafficUnit = model.TrafficUnit
	// EdgeKey identifies an edge.
	EdgeKey = model.EdgeKey
	// Relation is an edge's meaning.
	Relation = model.Relation
	// Status is an entity's health and why.
	Status = model.Status
	// StatusLevel orders statuses from unknown to down.
	StatusLevel = model.StatusLevel
	// Event is something that happened to an entity.
	Event = model.Event
	// Severity orders events from debug to critical.
	Severity = model.Severity
	// Value is an attribute's value.
	Value = model.Value
	// Type is a value's type.
	Type = model.Type
	// Unit is a metric's or attribute's unit.
	Unit = model.Unit
)

// Kinds shared across modules; a module may use its own.
const (
	KindHost      = model.KindHost
	KindService   = model.KindService
	KindContainer = model.KindContainer
	KindPod       = model.KindPod
	KindNode      = model.KindNode
	KindCluster   = model.KindCluster
	KindDatabase  = model.KindDatabase
	KindTable     = model.KindTable
	KindQueue     = model.KindQueue
	KindDisk      = model.KindDisk
	KindInterface = model.KindInterface
	KindProcess   = model.KindProcess
	KindPerson    = model.KindPerson
	KindTeam      = model.KindTeam
	KindRepo      = model.KindRepo
	KindAlert     = model.KindAlert
)

// Relations.
const (
	RelRunsOn    = model.RelRunsOn
	RelDependsOn = model.RelDependsOn
	RelTalksTo   = model.RelTalksTo
	RelMemberOf  = model.RelMemberOf
	RelOwns      = model.RelOwns
	RelParentOf  = model.RelParentOf
	RelSameAs    = model.RelSameAs
)

// Status levels.
const (
	StatusUnknown = model.StatusUnknown
	StatusOK      = model.StatusOK
	StatusWarn    = model.StatusWarn
	StatusCrit    = model.StatusCrit
	StatusDown    = model.StatusDown
)

// Severities.
const (
	SevDebug    = model.SevDebug
	SevInfo     = model.SevInfo
	SevWarn     = model.SevWarn
	SevError    = model.SevError
	SevCritical = model.SevCritical
)

// Value types.
const (
	TypeNone   = model.TypeNone
	TypeString = model.TypeString
	TypeNumber = model.TypeNumber
	TypeBool   = model.TypeBool
	TypeTime   = model.TypeTime
	TypeList   = model.TypeList
)

// Units.
const (
	UnitNone    = model.UnitNone
	UnitBytes   = model.UnitBytes
	UnitBytesPS = model.UnitBytesPS
	UnitBits    = model.UnitBits
	UnitBitsPS  = model.UnitBitsPS
	UnitPercent = model.UnitPercent
	UnitRatio   = model.UnitRatio
	UnitSeconds = model.UnitSeconds
	UnitCount   = model.UnitCount
	UnitPerSec  = model.UnitPerSec
)

// Traffic units.
const (
	TrafficNone     = model.TrafficNone
	TrafficRequests = model.TrafficRequests
	TrafficBytes    = model.TrafficBytes
	TrafficMessages = model.TrafficMessages
)

// ParseTrafficUnit reads a traffic unit's name: requests, bytes or messages.
func ParseTrafficUnit(s string) (TrafficUnit, error) { return model.ParseTrafficUnit(s) }

// NewEntityRef builds the ref for a native ID in a module instance.
func NewEntityRef(instance string, kind Kind, native string) (EntityRef, error) {
	return model.NewEntityRef(instance, kind, native)
}

// String is a string value.
func String(s string) Value { return model.String(s) }

// Number is a numeric value.
func Number(f float64) Value { return model.Number(f) }

// Bool is a boolean value.
func Bool(b bool) Value { return model.Bool(b) }

// Time is a time value.
func Time(t time.Time) Value { return model.Time(t) }

// List is a list value.
func List(vs ...Value) Value { return model.List(vs...) }

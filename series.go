package sdk

import (
	"time"
	"wayseer/pkg/sdk/data"
)

// Series and the queries over them.
type (
	// SeriesRef names a series: an entity and a metric.
	SeriesRef = data.SeriesRef
	// SeriesQuery asks for series over a window at a step.
	SeriesQuery = data.SeriesQuery
	// Series is one series' points.
	Series = data.Series
	// Point is one sample: Unix nanoseconds and a value.
	Point = data.Point
	// TimeWindow is a span of time.
	TimeWindow = data.TimeWindow
	// Aggregation says how points in one step combine.
	Aggregation = data.Aggregation
	// Ring keeps a series' most recent points.
	Ring = data.Ring
)

// Aggregations.
const (
	AggNone = data.AggNone
	AggAvg  = data.AggAvg
	AggMax  = data.AggMax
	AggMin  = data.AggMin
	AggSum  = data.AggSum
	AggP95  = data.AggP95
)

// NewRing keeps at most n points.
func NewRing(n int) *Ring { return data.NewRing(n) }

// Downsample keeps at most pixels points of in (sorted by time) inside w; spikes survive.
func Downsample(in []Point, w TimeWindow, pixels int) []Point { return data.Downsample(in, w, pixels) }

// StepFor is the smallest round step giving no more samples than pixels across w.
func StepFor(w TimeWindow, pixels int) time.Duration { return data.StepFor(w, pixels) }

package data

import (
	"fmt"
	"time"

	"wayseer.dev/sdk/model"
)

// SeriesRef names one metric of one entity; Metric is a canonical name from a module's catalogue.
type SeriesRef struct {
	Entity model.EntityRef
	Metric string
}

// Point is one sample: T is UTC Unix nanoseconds, kept as an integer to stay compact.
type Point struct {
	T int64
	V float64
}

// Time returns the sample time in UTC.
func (p Point) Time() time.Time { return time.Unix(0, p.T).UTC() }

// Series is the samples of one SeriesRef in time order.
type Series struct {
	Ref    SeriesRef
	Unit   model.Unit
	Points []Point
}

// TimeWindow is the half-open interval [From, To).
type TimeWindow struct {
	From, To time.Time
}

// Span is the window's length.
func (w TimeWindow) Span() time.Duration { return w.To.Sub(w.From) }

// Contains reports whether the sample time t (Unix nanoseconds) falls inside the window.
func (w TimeWindow) Contains(t int64) bool { return t >= w.From.UnixNano() && t < w.To.UnixNano() }

// key identifies the window's instants, ignoring zone and monotonic clock readings.
func (w TimeWindow) key() [2]int64 { return [2]int64{w.From.UnixNano(), w.To.UnixNano()} }

// Aggregation combines samples within a step or across series.
type Aggregation uint8

// The aggregations every module must understand.
const (
	AggNone Aggregation = iota
	AggAvg
	AggMax
	AggMin
	AggSum
	AggP95
)

var aggNames = [...]string{"none", "avg", "max", "min", "sum", "p95"}

func (a Aggregation) String() string {
	if int(a) < len(aggNames) {
		return aggNames[a]
	}
	return fmt.Sprintf("Aggregation(%d)", a)
}

// ParseAggregation reads a name written by Aggregation.String.
func ParseAggregation(s string) (Aggregation, error) {
	for i, n := range aggNames {
		if n == s {
			return Aggregation(i), nil
		}
	}
	return AggNone, fmt.Errorf("aggregation %q (want none, avg, max, min, sum or p95)", s)
}

// SeriesQuery is the structured query every module translates (§6.2).
type SeriesQuery struct {
	Entities []model.EntityRef // explicit entities; otherwise Filter selects them
	Filter   Filter
	Metrics  []string
	Window   TimeWindow
	Step     time.Duration // resolution hint; see StepFor
	Agg      Aggregation
	Native   string // module-native passthrough for power users; never set by the NL layer
	// Top, when above zero, asks a module that ranks (module.TopRanker) for at most that many
	// entities per metric: those with the highest newest value in the window, leaving out
	// entities without the metric. A module that does not rank ignores it.
	Top int
}

// niceSteps are the step sizes StepFor rounds up to, so nearby windows share cache entries.
var niceSteps = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second,
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute,
	time.Hour, 2 * time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// StepFor is the smallest nice step giving no more samples than pixels across w.
func StepFor(w TimeWindow, pixels int) time.Duration {
	raw := w.Span() / time.Duration(max(pixels, 1))
	if raw*time.Duration(max(pixels, 1)) < w.Span() {
		raw++
	}
	if raw < time.Second {
		return niceBelowSecond(raw)
	}
	for _, s := range niceSteps {
		if s >= raw {
			return s
		}
	}
	days := (raw + 24*time.Hour - 1) / (24 * time.Hour)
	return days * 24 * time.Hour
}

// niceBelowSecond rounds d up to 1, 2 or 5 times a power of ten.
func niceBelowSecond(d time.Duration) time.Duration {
	for p := time.Duration(1); ; p *= 10 {
		for _, m := range []time.Duration{1, 2, 5} {
			if m*p >= d {
				return m * p
			}
		}
	}
}

// Align widens w to whole multiples of step, so windows that move less than a step share a
// cache entry. Its times are UTC without monotonic readings, so aligned windows compare with ==.
func (w TimeWindow) Align(step time.Duration) TimeWindow {
	s := int64(max(step, 1))
	from := floorDiv(w.From.UnixNano(), s) * s
	to := -floorDiv(-w.To.UnixNano(), s) * s
	return TimeWindow{From: time.Unix(0, from).UTC(), To: time.Unix(0, to).UTC()}
}

// floorDiv is a/b rounded toward negative infinity, for b > 0.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}

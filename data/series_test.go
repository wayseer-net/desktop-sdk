package data

import (
	"testing"
	"time"
)

func TestStepForNeverExceedsPixels(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, span := range []time.Duration{time.Second, time.Minute, 7 * time.Minute, time.Hour, 26 * time.Hour, 400 * 24 * time.Hour} {
		for _, px := range []int{1, 3, 640, 1920, 3840} {
			w := TimeWindow{From: from, To: from.Add(span)}
			step := StepFor(w, px)
			if step <= 0 || int(span/step) > px {
				t.Errorf("span %v px %d: step %v gives %d points", span, px, step, span/step)
			}
			if StepFor(TimeWindow{From: from.Add(time.Second), To: from.Add(span + time.Second)}, px) != step {
				t.Errorf("span %v px %d: step changed when the window slid", span, px)
			}
		}
	}
}

func TestStepForIsNice(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	cases := map[time.Duration]time.Duration{time.Hour: 5 * time.Second, 24 * time.Hour: 2 * time.Minute, time.Minute: 100 * time.Millisecond}
	for span, want := range cases {
		if got := StepFor(TimeWindow{From: from, To: from.Add(span)}, 1000); got != want {
			t.Errorf("span %v: step %v, want %v", span, got, want)
		}
	}
}

func TestTimeWindowKeyIgnoresZoneAndMonotonic(t *testing.T) {
	now := time.Now()
	a := TimeWindow{From: now, To: now.Add(time.Hour)}
	b := TimeWindow{From: now.Round(0).In(time.FixedZone("x", 3600)), To: now.Add(time.Hour).UTC()}
	if a.key() != b.key() {
		t.Error("the same instants should give the same key")
	}
}

func TestAggregationParse(t *testing.T) {
	for _, a := range []Aggregation{AggNone, AggAvg, AggMax, AggMin, AggSum, AggP95} {
		if got, err := ParseAggregation(a.String()); err != nil || got != a {
			t.Errorf("%v: %v, %v", a, got, err)
		}
	}
	if _, err := ParseAggregation("median"); err == nil {
		t.Error("unknown aggregation should fail")
	}
}

func TestAlignCoversTheWindowOnStepBoundaries(t *testing.T) {
	step := 5 * time.Second
	base := time.Unix(1_800_000_000, 0)
	for _, w := range []TimeWindow{
		{From: base.Add(1300 * time.Millisecond), To: base.Add(15*time.Minute + 2*time.Second)},
		{From: base, To: base.Add(time.Minute)},
		{From: base.Add(-7 * time.Second), To: base.Add(-time.Second)},
	} {
		a := w.Align(step)
		if a.From.After(w.From) || a.To.Before(w.To) {
			t.Errorf("%v aligned to %v leaves part of it out", w, a)
		}
		if a.From.UnixNano()%int64(step) != 0 || a.To.UnixNano()%int64(step) != 0 {
			t.Errorf("%v aligned to %v, off the %v boundaries", w, a, step)
		}
		if a.Span() >= w.Span()+2*step {
			t.Errorf("%v aligned to %v, wider than it needs", w, a)
		}
	}
}

func TestNearbyWindowsAlignAlike(t *testing.T) {
	step := 5 * time.Second
	base := time.Unix(1_800_000_000, 0)
	w := TimeWindow{From: base.Add(time.Second), To: base.Add(15*time.Minute + time.Second)}
	moved := TimeWindow{From: w.From.Add(3 * time.Second), To: w.To.Add(3 * time.Second)}
	if w.Align(step) != moved.Align(step) {
		t.Errorf("windows 3 s apart align to %v and %v; want one cache entry", w.Align(step), moved.Align(step))
	}
}

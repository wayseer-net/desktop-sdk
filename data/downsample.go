package data

import "math"

// Downsample returns at most pixels points of in (sorted by time) that fall inside w. Each
// two-pixel bucket keeps its minimum and maximum in time order, so spikes survive.
func Downsample(in []Point, w TimeWindow, pixels int) []Point {
	lo, hi := w.From.UnixNano(), w.To.UnixNano()
	in = clip(in, lo, hi)
	if len(in) <= pixels || pixels <= 0 {
		if pixels <= 0 {
			return nil
		}
		return in
	}
	if pixels == 1 {
		return appendExtremes(nil, in, false)
	}
	buckets := pixels / 2
	out := make([]Point, 0, pixels)
	span := float64(hi - lo)
	start := 0
	for b := range buckets {
		end := start
		limit := lo + int64(float64(b+1)*span/float64(buckets))
		for end < len(in) && (in[end].T < limit || b == buckets-1) {
			end++
		}
		out = appendExtremes(out, in[start:end], true)
		start = end
	}
	return out
}

// clip returns the sub-slice of sorted points with lo <= T < hi.
func clip(in []Point, lo, hi int64) []Point {
	start := 0
	for start < len(in) && in[start].T < lo {
		start++
	}
	end := len(in)
	for end > start && in[end-1].T >= hi {
		end--
	}
	return in[start:end]
}

// appendExtremes appends the maximum of ps, and with both also the minimum, in time order, ignoring NaN.
func appendExtremes(out, ps []Point, both bool) []Point {
	mn, mx := -1, -1
	for i, p := range ps {
		if math.IsNaN(p.V) {
			continue
		}
		if mn < 0 || p.V < ps[mn].V {
			mn = i
		}
		if mx < 0 || p.V > ps[mx].V {
			mx = i
		}
	}
	switch {
	case mx < 0:
		return out
	case !both || mn == mx:
		return append(out, ps[mx])
	case mn < mx:
		return append(out, ps[mn], ps[mx])
	}
	return append(out, ps[mx], ps[mn])
}

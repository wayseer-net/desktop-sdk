package data

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func window(n int) TimeWindow {
	return TimeWindow{From: time.Unix(0, 0), To: time.Unix(int64(n), 0)}
}

func series(rng *rand.Rand, n int) []Point {
	out := make([]Point, n)
	for i := range out {
		out[i] = Point{T: int64(i) * int64(time.Second), V: rng.NormFloat64()}
	}
	return out
}

func TestDownsampleKeepsExtremesWithinPixels(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := range 300 {
		n, px := rng.IntN(5000), 1+rng.IntN(800)
		in := series(rng, n)
		if n > 0 && trial%3 == 0 {
			in[rng.IntN(n)].V = 1000 // spike
		}
		out := Downsample(in, window(n), px)
		if len(out) > px {
			t.Fatalf("n %d px %d: %d points", n, px, len(out))
		}
		if !slices.IsSortedFunc(out, func(a, b Point) int { return int(a.T - b.T) }) {
			t.Fatalf("out of order")
		}
		for _, p := range out {
			if !slices.Contains(in, p) {
				t.Fatalf("invented point %+v", p)
			}
		}
		if n > 0 && px > 1 && (extreme(out, math.Max) != extreme(in, math.Max) || extreme(out, math.Min) != extreme(in, math.Min)) {
			t.Fatalf("n %d px %d: lost an extreme", n, px)
		}
	}
}

func TestDownsampleShortSeriesUnchanged(t *testing.T) {
	in := series(rand.New(rand.NewPCG(1, 1)), 10)
	if out := Downsample(in, window(10), 100); !slices.Equal(out, in) {
		t.Error("a series that fits should come back as is")
	}
}

func TestDownsampleDropsPointsOutsideWindow(t *testing.T) {
	in := []Point{{T: -1, V: 9}, {T: 0, V: 1}, {T: int64(time.Second), V: 2}, {T: int64(5 * time.Second), V: 9}}
	out := Downsample(in, window(2), 100)
	if len(out) != 2 || out[0].V != 1 || out[1].V != 2 {
		t.Errorf("out = %+v", out)
	}
}

func TestDownsampleSkipsNaN(t *testing.T) {
	in := series(rand.New(rand.NewPCG(1, 2)), 1000)
	for i := range 500 {
		in[i].V = math.NaN()
	}
	for _, p := range Downsample(in, window(1000), 50) {
		if math.IsNaN(p.V) {
			t.Fatal("NaN chosen as an extreme")
		}
	}
}

func extreme(ps []Point, pick func(a, b float64) float64) float64 {
	v := ps[0].V
	for _, p := range ps {
		v = pick(v, p.V)
	}
	return v
}

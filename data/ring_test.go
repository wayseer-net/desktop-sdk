package data

import (
	"slices"
	"testing"
	"time"
)

func TestRingKeepsNewestInOrder(t *testing.T) {
	r := NewRing(3)
	for i := range int64(5) {
		r.Add(Point{T: i, V: float64(i)})
	}
	all := TimeWindow{From: time.Unix(0, 0), To: time.Unix(0, 100)}
	if got, want := r.In(all), []Point{{2, 2}, {3, 3}, {4, 4}}; !slices.Equal(got, want) {
		t.Errorf("In = %v; want %v", got, want)
	}
	if got := r.In(TimeWindow{From: time.Unix(0, 3), To: time.Unix(0, 4)}); !slices.Equal(got, []Point{{3, 3}}) {
		t.Errorf("In [3,4) = %v", got)
	}
}

func TestRingDropsPointsThatGoBackInTime(t *testing.T) {
	r := NewRing(4)
	r.Add(Point{T: 5, V: 1})
	r.Add(Point{T: 5, V: 2})
	r.Add(Point{T: 4, V: 3})
	if got := r.In(TimeWindow{From: time.Unix(0, 0), To: time.Unix(0, 10)}); !slices.Equal(got, []Point{{5, 1}}) {
		t.Errorf("In = %v; want only the first point", got)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d", r.Len())
	}
}

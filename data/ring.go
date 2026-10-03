package data

// Ring keeps the newest points of one series in fixed memory, for modules that sample locally.
type Ring struct {
	points []Point
	total  int
}

// NewRing makes a ring holding up to n points; n must be positive.
func NewRing(n int) *Ring { return &Ring{points: make([]Point, max(n, 1))} }

// Add appends p unless it is not newer than the last point, which would break the order.
func (r *Ring) Add(p Point) {
	if r.total > 0 && p.T <= r.points[(r.total-1)%len(r.points)].T {
		return
	}
	r.points[r.total%len(r.points)] = p
	r.total++
}

// Len is the number of points held.
func (r *Ring) Len() int { return min(r.total, len(r.points)) }

// In returns a copy of the points inside w, oldest first.
func (r *Ring) In(w TimeWindow) []Point {
	n := len(r.points)
	var out []Point
	for i := r.total - r.Len(); i < r.total; i++ {
		if p := r.points[i%n]; w.Contains(p.T) {
			out = append(out, p)
		}
	}
	return out
}

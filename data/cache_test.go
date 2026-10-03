package data

import (
	"testing"
	"time"
)

func cacheKey(metric string) CacheKey {
	from := time.Unix(1000, 0)
	return NewCacheKey(SeriesRef{Entity: "m/host/a", Metric: metric}, TimeWindow{From: from, To: from.Add(time.Hour)}, time.Minute)
}

func pts(n int) []Point { return make([]Point, n) }

func TestCacheEvictsLeastRecentlyUsedByPoints(t *testing.T) {
	c := NewCache(10)
	now := time.Unix(0, 0)
	c.Put(cacheKey("a"), Series{Points: pts(4)}, 0, now)
	c.Put(cacheKey("b"), Series{Points: pts(4)}, 0, now)
	if _, ok := c.Get(cacheKey("a"), now); !ok { // a is now most recent
		t.Fatal("a missing")
	}
	c.Put(cacheKey("c"), Series{Points: pts(4)}, 0, now)
	if _, ok := c.Get(cacheKey("b"), now); ok {
		t.Error("b should have been evicted")
	}
	for _, m := range []string{"a", "c"} {
		if _, ok := c.Get(cacheKey(m), now); !ok {
			t.Errorf("%s evicted", m)
		}
	}
	if c.Points() != 8 || c.Len() != 2 {
		t.Errorf("points %d len %d", c.Points(), c.Len())
	}
}

func TestCacheTTL(t *testing.T) {
	c := NewCache(100)
	now := time.Unix(0, 0)
	c.Put(cacheKey("live"), Series{Points: pts(1)}, 10*time.Second, now)
	c.Put(cacheKey("past"), Series{Points: pts(1)}, 0, now)
	later := now.Add(11 * time.Second)
	if _, ok := c.Get(cacheKey("live"), now.Add(9*time.Second)); !ok {
		t.Error("live expired early")
	}
	if _, ok := c.Get(cacheKey("live"), later); ok {
		t.Error("live should have expired")
	}
	if _, ok := c.Get(cacheKey("past"), later.Add(24*time.Hour)); !ok {
		t.Error("a zero TTL never expires")
	}
	if c.Len() != 1 || c.Points() != 1 {
		t.Errorf("expired entry not dropped: len %d points %d", c.Len(), c.Points())
	}
}

func TestCacheReplaceAndOversize(t *testing.T) {
	c := NewCache(5)
	now := time.Unix(0, 0)
	c.Put(cacheKey("a"), Series{Points: pts(2)}, 0, now)
	c.Put(cacheKey("a"), Series{Points: pts(3)}, 0, now)
	if c.Points() != 3 || c.Len() != 1 {
		t.Errorf("replace: points %d len %d", c.Points(), c.Len())
	}
	c.Put(cacheKey("big"), Series{Points: pts(6)}, 0, now)
	if _, ok := c.Get(cacheKey("big"), now); ok {
		t.Error("a series larger than the cache should not be stored")
	}
	if _, ok := c.Get(cacheKey("a"), now); !ok {
		t.Error("an oversize put should not evict others")
	}
}

func TestCacheKeysDifferByStep(t *testing.T) {
	c := NewCache(100)
	now := time.Unix(0, 0)
	k := cacheKey("a")
	c.Put(k, Series{Points: pts(1)}, 0, now)
	k2 := NewCacheKey(k.Ref, TimeWindow{From: time.Unix(1000, 0), To: time.Unix(4600, 0)}, time.Second)
	if _, ok := c.Get(k2, now); ok {
		t.Error("different step should miss")
	}
}

func TestRemovingASourceForgetsItsSeries(t *testing.T) {
	c := NewCache(100)
	now := time.Unix(0, 0)
	other := NewCacheKey(SeriesRef{Entity: "n/host/b", Metric: "cpu"}, TimeWindow{}, time.Minute)
	c.Put(cacheKey("cpu"), Series{Points: pts(3)}, 0, now)
	c.Put(cacheKey("mem"), Series{Points: pts(4)}, 0, now)
	c.Put(other, Series{Points: pts(5)}, 0, now)
	c.Forget("m")
	if _, ok := c.Get(cacheKey("cpu"), now); ok {
		t.Error("the removed source's series is still cached")
	}
	if _, ok := c.Get(other, now); !ok {
		t.Error("another source's series went too")
	}
	if c.Len() != 1 || c.Points() != 5 {
		t.Errorf("len, points = %d, %d; want 1, 5", c.Len(), c.Points())
	}
}

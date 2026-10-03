package data

import (
	"container/list"
	"sync"
	"time"

	"wayseer.dev/sdk/model"
)

// CacheKey identifies a cached series by reference, window instants and step.
type CacheKey struct {
	Ref    SeriesRef
	window [2]int64
	step   time.Duration
}

// NewCacheKey builds the key for ref over w at step.
func NewCacheKey(ref SeriesRef, w TimeWindow, step time.Duration) CacheKey {
	return CacheKey{Ref: ref, window: w.key(), step: step}
}

// Cache is an LRU of series bounded by total points, with per-entry TTLs from module hints.
// It is safe for concurrent use; cached series must not be modified.
type Cache struct {
	mu        sync.Mutex
	maxPoints int
	points    int
	order     *list.List // front is most recent; values are *cacheEntry
	byKey     map[CacheKey]*list.Element
}

type cacheEntry struct {
	key     CacheKey
	series  Series
	expires time.Time // zero never expires
}

// NewCache returns a cache holding at most maxPoints samples in total.
func NewCache(maxPoints int) *Cache {
	return &Cache{maxPoints: maxPoints, order: list.New(), byKey: map[CacheKey]*list.Element{}}
}

// Get returns the series under k if present and unexpired at now.
func (c *Cache) Get(k CacheKey, now time.Time) (Series, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.byKey[k]
	if !ok {
		return Series{}, false
	}
	e := el.Value.(*cacheEntry)
	if !e.expires.IsZero() && !now.Before(e.expires) {
		c.remove(el)
		return Series{}, false
	}
	c.order.MoveToFront(el)
	return e.series, true
}

// Put stores s under k for ttl (zero means until evicted), evicting least recently used entries.
// A series larger than the whole cache is not stored.
func (c *Cache) Put(k CacheKey, s Series, ttl time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(s.Points) > c.maxPoints {
		return
	}
	if el, ok := c.byKey[k]; ok {
		c.remove(el)
	}
	e := &cacheEntry{key: k, series: s}
	if ttl > 0 {
		e.expires = now.Add(ttl)
	}
	c.byKey[k] = c.order.PushFront(e)
	c.points += len(s.Points)
	for c.points > c.maxPoints {
		c.remove(c.order.Back())
	}
}

// Forget drops every cached series of src's entities.
func (c *Cache) Forget(src model.ModuleID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, el := range c.byKey {
		if model.ModuleID(k.Ref.Entity.Instance()) == src {
			c.remove(el)
		}
	}
}

func (c *Cache) remove(el *list.Element) {
	e := c.order.Remove(el).(*cacheEntry)
	delete(c.byKey, e.key)
	c.points -= len(e.series.Points)
}

// Len is the number of cached series.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Points is the total number of cached samples.
func (c *Cache) Points() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.points
}

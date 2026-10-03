package data

import (
	"sync"
	"wayseer/pkg/sdk/model"
)

// SameAsIn links entities through the same_as groups of whatever world reads, remembering each
// group asked about until the world changes. It is safe for concurrent use.
func SameAsIn(world func() *model.Snapshot) Linked {
	var (
		mu     sync.Mutex
		seen   *model.Snapshot
		groups map[model.EntityRef]map[model.EntityRef]bool
	)
	return func(e *model.Entity, ref model.EntityRef) bool {
		w := world()
		if w == nil {
			return false
		}
		mu.Lock()
		defer mu.Unlock()
		if w != seen {
			seen, groups = w, map[model.EntityRef]map[model.EntityRef]bool{}
		}
		g, ok := groups[ref]
		if !ok {
			g = map[model.EntityRef]bool{}
			for _, r := range w.SameAs(ref) {
				g[r] = true
			}
			groups[ref] = g
		}
		return g[e.Ref]
	}
}

package serve

import (
	"testing"
	"time"
)

// TestAModuleLeavesOnceItsParentHasGone has the parent change, as it does when the app dies and
// the system adopts the module, and finds the module told to go.
func TestAModuleLeavesOnceItsParentHasGone(t *testing.T) {
	ppids := []int{4242, 4242, 4242, 1}
	asked := 0
	ppid := func() int {
		asked++
		return ppids[min(asked-1, len(ppids)-1)]
	}
	gone := make(chan struct{})
	go watchParent(ppid, time.Millisecond, func() { close(gone) })
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("the parent went and the module stayed")
	}
	if asked != len(ppids) {
		t.Errorf("asked for the parent %d times; want it told to go at the first change, the %dth", asked, len(ppids))
	}
}

package serve

import "time"

// parentPoll is how often a module checks that the app that started it is still there.
const parentPoll = time.Second

// watchParent calls gone once ppid reports another parent than at first, as it does on Unix
// when the app has died and the system adopted the module. On Windows the parent never changes;
// the app ends its modules there itself.
func watchParent(ppid func() int, every time.Duration, gone func()) {
	first := ppid()
	for range time.Tick(every) {
		if ppid() != first {
			gone()
			return
		}
	}
}

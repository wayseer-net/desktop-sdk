package module

import (
	"slices"

	"wayseer.dev/sdk/model"
)

// EventLog keeps a module's most recent events to answer EventQuery; it is not safe for
// concurrent use.
type EventLog struct {
	events []model.Event // oldest first
	cap    int
}

// NewEventLog keeps at most n events.
func NewEventLog(n int) *EventLog { return &EventLog{cap: n} }

// Add appends events in the order they happened, forgetting the oldest beyond the cap.
func (l *EventLog) Add(evs ...model.Event) {
	l.events = append(l.events, evs...)
	if extra := len(l.events) - l.cap; extra > 0 {
		l.events = slices.Delete(l.events, 0, extra)
	}
}

// Query returns the matching events oldest first; with a limit, the most recent ones. A nil
// log has none.
func (l *EventLog) Query(q EventQuery) []model.Event {
	var out []model.Event
	if l == nil {
		return nil
	}
	for i := len(l.events) - 1; i >= 0 && (q.Limit == 0 || len(out) < q.Limit); i-- {
		if e := &l.events[i]; q.Matches(e) {
			out = append(out, *e)
		}
	}
	slices.Reverse(out)
	return out
}

// Matches reports whether e passes every filter set in q.
func (q *EventQuery) Matches(e *model.Event) bool {
	switch {
	case e.Severity < q.MinSeverity:
		return false
	case len(q.Entities) > 0 && !slices.Contains(q.Entities, e.Entity):
		return false
	case len(q.Kinds) > 0 && !slices.Contains(q.Kinds, e.Kind):
		return false
	}
	return q.Window.From.IsZero() && q.Window.To.IsZero() || q.Window.Contains(e.At.UnixNano())
}

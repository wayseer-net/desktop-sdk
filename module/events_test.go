package module

import (
	"slices"
	"testing"
	"time"
	"wayseer/pkg/sdk/data"
	"wayseer/pkg/sdk/model"
)

func event(id string, at int64, sev model.Severity, kind string, ent model.EntityRef) model.Event {
	return model.Event{ID: id, At: time.Unix(at, 0), Severity: sev, Kind: kind, Entity: ent, Source: "t"}
}

func ids(evs []model.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.ID
	}
	return out
}

func TestEventLogForgetsTheOldest(t *testing.T) {
	l := NewEventLog(3)
	for i, id := range []string{"a", "b", "c", "d"} {
		l.Add(event(id, int64(i), model.SevInfo, "log", ""))
	}
	if got := ids(l.Query(EventQuery{})); !slices.Equal(got, []string{"b", "c", "d"}) {
		t.Errorf("kept %v, want [b c d]", got)
	}
}

func TestNilEventLogIsEmpty(t *testing.T) {
	var l *EventLog
	if got := l.Query(EventQuery{}); got != nil {
		t.Errorf("nil log returned %v", got)
	}
}

func TestEventLogQuery(t *testing.T) {
	a, b := host("a", "A").Ref, host("b", "B").Ref
	l := NewEventLog(10)
	l.Add(
		event("1", 10, model.SevInfo, "log", a),
		event("2", 20, model.SevError, "log", b),
		event("3", 30, model.SevWarn, "problem", a),
		event("4", 40, model.SevCritical, "log", a),
	)
	cases := []struct {
		name string
		q    EventQuery
		want []string
	}{
		{"all, oldest first", EventQuery{}, []string{"1", "2", "3", "4"}},
		{"severity", EventQuery{MinSeverity: model.SevWarn}, []string{"2", "3", "4"}},
		{"entity", EventQuery{Entities: []model.EntityRef{b}}, []string{"2"}},
		{"kind", EventQuery{Kinds: []string{"problem"}}, []string{"3"}},
		{"window", EventQuery{Window: data.TimeWindow{From: time.Unix(15, 0), To: time.Unix(35, 0)}}, []string{"2", "3"}},
		{"limit keeps the newest", EventQuery{Limit: 2, Kinds: []string{"log"}}, []string{"2", "4"}},
	}
	for _, c := range cases {
		if got := ids(l.Query(c.q)); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

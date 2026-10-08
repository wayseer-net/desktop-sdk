package data

import (
	"errors"
	"testing"
	"time"
)

func TestFreshnessTransitions(t *testing.T) {
	f := NewFreshness(30 * time.Second)
	t0 := time.Unix(1000, 0)
	if got := f.State("m", t0); got != FreshDisconnected {
		t.Errorf("unknown module = %v, want disconnected", got)
	}
	f.Seen(t0, "m")
	steps := []struct {
		at   time.Duration
		want State
	}{{0, FreshLive}, {30 * time.Second, FreshLive}, {30*time.Second + 1, FreshStale}}
	for _, s := range steps {
		if got := f.State("m", t0.Add(s.at)); got != s.want {
			t.Errorf("at +%v = %v, want %v", s.at, got, s.want)
		}
	}
	f.Seen(t0.Add(time.Minute), "m")
	if got := f.State("m", t0.Add(time.Minute)); got != FreshLive {
		t.Errorf("after new data = %v, want live", got)
	}
}

func TestFreshnessHealthOverridesAge(t *testing.T) {
	f := NewFreshness(30 * time.Second)
	now := time.Unix(1000, 0)
	f.Seen(now, "m")
	f.SetHealth("m", Health{Err: errors.New("boom")})
	if got := f.State("m", now); got != FreshError {
		t.Errorf("= %v, want error", got)
	}
	if f.Health("m").Err == nil {
		t.Error("error not reported")
	}
	f.SetHealth("m", Health{Disconnected: true})
	if got := f.State("m", now); got != FreshDisconnected {
		t.Errorf("= %v, want disconnected", got)
	}
	f.SetHealth("m", Health{})
	if got := f.State("m", now); got != FreshLive {
		t.Errorf("recovered = %v, want live", got)
	}
}

func TestFreshnessHealthyButSilentIsStale(t *testing.T) {
	f := NewFreshness(30 * time.Second)
	f.SetHealth("m", Health{})
	if got := f.State("m", time.Unix(0, 0)); got != FreshStale {
		t.Errorf("= %v, want stale", got)
	}
}

func TestFreshnessModulesSorted(t *testing.T) {
	f := NewFreshness(time.Second)
	f.Seen(time.Unix(0, 0), "b")
	f.SetHealth("a", Health{})
	if got := f.Modules(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("modules = %v", got)
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{FreshLive: "live", FreshStale: "stale", FreshDisconnected: "disconnected", FreshError: "error"} {
		if s.String() != want {
			t.Errorf("%d = %q", s, s.String())
		}
	}
}

func TestFreshnessFollowsEachModulesPace(t *testing.T) {
	cases := []struct {
		name      string
		gap       time.Duration // between the module's last two arrivals
		liveUntil time.Duration // after the last arrival
	}{
		{"slower than the threshold waits two gaps", 38 * time.Second, 76 * time.Second},
		{"faster than the threshold keeps it", time.Second, 30 * time.Second},
		{"a long outage counts up to five minutes", time.Hour, 10 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := NewFreshness(30 * time.Second)
			t0 := time.Unix(1000, 0)
			last := t0.Add(c.gap)
			f.Seen(t0, "m")
			f.Seen(last, "m")
			if got := f.State("m", last.Add(c.liveUntil)); got != FreshLive {
				t.Errorf("at +%v = %v, want live", c.liveUntil, got)
			}
			if got := f.State("m", last.Add(c.liveUntil+1)); got != FreshStale {
				t.Errorf("at +%v = %v, want stale", c.liveUntil+1, got)
			}
		})
	}
}

func TestFreshnessWaitsTwoOfADeclaredPace(t *testing.T) {
	cases := []struct {
		name      string
		pace      time.Duration
		gap       time.Duration // between the last two arrivals; 0 for one arrival so far
		liveUntil time.Duration
	}{
		{"from the first arrival", 5 * time.Minute, 0, 10 * time.Minute},
		{"beyond five minutes, as declared", time.Hour, 0, 2 * time.Hour},
		{"a slower measured gap still counts", time.Minute, 3 * time.Minute, 6 * time.Minute},
		{"below the threshold keeps it", time.Second, 0, 30 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := NewFreshness(30 * time.Second)
			f.SetHealth("m", Health{Pace: c.pace})
			last := time.Unix(1000, 0)
			if c.gap > 0 {
				f.Seen(last.Add(-c.gap), "m")
			}
			f.Seen(last, "m")
			if got := f.State("m", last.Add(c.liveUntil)); got != FreshLive {
				t.Errorf("at +%v = %v, want live", c.liveUntil, got)
			}
			if got := f.State("m", last.Add(c.liveUntil+1)); got != FreshStale {
				t.Errorf("at +%v = %v, want stale", c.liveUntil+1, got)
			}
		})
	}
}

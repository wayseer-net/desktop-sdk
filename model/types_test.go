package model

import (
	"math"
	"testing"
)

func TestStatusOrdering(t *testing.T) {
	order := []StatusLevel{StatusUnknown, StatusOK, StatusWarn, StatusCrit, StatusDown}
	for i := 1; i < len(order); i++ {
		if !order[i].Worse(order[i-1]) {
			t.Errorf("%v should be worse than %v", order[i], order[i-1])
		}
	}
	if StatusCrit.String() != "crit" || StatusLevel(99).String() != "status(99)" {
		t.Error("status names")
	}
}

func TestSeverityNames(t *testing.T) {
	for s, want := range map[Severity]string{SevDebug: "debug", SevInfo: "info", SevWarn: "warn", SevError: "error", SevCritical: "critical"} {
		if s.String() != want {
			t.Errorf("%d = %q, want %q", s, s.String(), want)
		}
	}
}

func TestEntityValidate(t *testing.T) {
	ref, _ := NewEntityRef("lh", KindHost, "me")
	good := Entity{Ref: ref, Kind: KindHost, Name: "me", Source: "lh"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, e := range map[string]Entity{
		"kind mismatch":   {Ref: ref, Kind: KindDisk, Source: "lh"},
		"source mismatch": {Ref: ref, Kind: KindHost, Source: "other"},
		"bad ref":         {Ref: "nope", Kind: KindHost, Source: "lh"},
	} {
		if e.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestEdgeValidate(t *testing.T) {
	a, _ := NewEntityRef("lh", KindProcess, "1")
	b, _ := NewEntityRef("lh", KindHost, "me")
	if err := (Edge{From: a, To: b, Rel: RelRunsOn, Source: "lh"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (Edge{From: a, To: a, Rel: RelRunsOn, Source: "lh"}).Validate() == nil {
		t.Error("self edge accepted")
	}
	if (Edge{From: a, To: b, Rel: "Bad Rel", Source: "lh"}).Validate() == nil {
		t.Error("bad relation accepted")
	}
}

func TestUnitValidate(t *testing.T) {
	for _, u := range []Unit{UnitNone, UnitBytes, UnitBytesPS, UnitBits, UnitBitsPS, UnitPercent, UnitRatio, UnitSeconds, UnitCount, UnitPerSec} {
		if err := u.Validate(); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
	for _, u := range []Unit{"byte", "ms", "Percent"} {
		if err := u.Validate(); err == nil {
			t.Errorf("%q validated", u)
		}
	}
}

func TestTrafficValidate(t *testing.T) {
	for _, tr := range []Traffic{{}, {Rate: 0, Unit: TrafficRequests}, {Rate: 12.5, Unit: TrafficBytes}, {Rate: 3, Unit: TrafficMessages}} {
		if err := tr.Validate(); err != nil {
			t.Errorf("%+v: %v", tr, err)
		}
	}
	for _, tr := range []Traffic{
		{Rate: -1, Unit: TrafficRequests},
		{Rate: math.NaN(), Unit: TrafficRequests},
		{Rate: math.Inf(1), Unit: TrafficBytes},
		{Rate: 1, Unit: 9},
		{Rate: 1},
	} {
		if err := tr.Validate(); err == nil {
			t.Errorf("%+v validated", tr)
		}
	}
}

func TestAnEdgeWithBadTrafficIsInvalid(t *testing.T) {
	a, _ := NewEntityRef("lh", KindService, "a")
	b, _ := NewEntityRef("lh", KindService, "b")
	e := Edge{From: a, To: b, Rel: RelTalksTo, Source: "lh", Traffic: Traffic{Rate: 4, Unit: TrafficRequests}}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Traffic.Rate = -4
	if e.Validate() == nil {
		t.Error("a negative rate was accepted")
	}
}

func TestTrafficUnitsReadAndWriteTheirNames(t *testing.T) {
	for _, u := range []TrafficUnit{TrafficRequests, TrafficBytes, TrafficMessages} {
		b, _ := u.MarshalText()
		var got TrafficUnit
		if err := got.UnmarshalText(b); err != nil || got != u {
			t.Errorf("%v read back as %v, %v", u, got, err)
		}
	}
	for _, s := range []string{"", "packets", "Requests"} {
		if _, err := ParseTrafficUnit(s); err == nil {
			t.Errorf("%q was read", s)
		}
	}
}

func TestPlaceValidate(t *testing.T) {
	for _, p := range []Place{{}, At(0, 0), At(52.37, 4.9), At(-90, -180), At(90, 180)} {
		if err := p.Validate(); err != nil {
			t.Errorf("%+v: %v", p, err)
		}
	}
	nan := float32(math.NaN())
	for _, p := range []Place{
		At(91, 0), At(-90.5, 0), At(0, 180.5), At(0, -181),
		At(nan, 0), At(0, nan), At(float32(math.Inf(1)), 0),
		{Lat: 10, Lon: 10},
	} {
		if err := p.Validate(); err == nil {
			t.Errorf("%+v validated", p)
		}
	}
}

func TestAnEntityWithABadPlaceIsInvalid(t *testing.T) {
	ref, _ := NewEntityRef("lh", KindHost, "a")
	e := Entity{Ref: ref, Kind: KindHost, Source: "lh", Place: At(1.35, 103.82)}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Place.Lat = 100
	if e.Validate() == nil {
		t.Error("a latitude of 100 was accepted")
	}
}

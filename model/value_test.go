package model

import (
	"math"
	"testing"
	"time"
	"unsafe"
)

func TestValueAccessors(t *testing.T) {
	at := time.Date(2026, 9, 25, 12, 0, 0, 123, time.UTC)
	if v := String("x"); v.Type() != TypeString || v.Str() != "x" {
		t.Errorf("string %+v", v)
	}
	if v := Number(2.5); v.Type() != TypeNumber || v.Num() != 2.5 {
		t.Errorf("number %+v", v)
	}
	if v := Bool(true); v.Type() != TypeBool || !v.Bool() {
		t.Errorf("bool %+v", v)
	}
	if v := Time(at); v.Type() != TypeTime || !v.Time().Equal(at) {
		t.Errorf("time %v", v.Time())
	}
	if v := List(Number(1), String("a")); v.Type() != TypeList || len(v.List()) != 2 {
		t.Errorf("list %+v", v)
	}
	if (Value{}).Type() != TypeNone {
		t.Error("zero value should be none")
	}
}

func TestValueEqual(t *testing.T) {
	eq := [][2]Value{
		{String("a"), String("a")},
		{List(Number(1), Bool(false)), List(Number(1), Bool(false))},
		{Time(time.Unix(5, 0)), Time(time.Unix(5, 0).In(time.FixedZone("x", 3600)))},
	}
	for _, p := range eq {
		if !p[0].Equal(p[1]) {
			t.Errorf("%v != %v", p[0], p[1])
		}
	}
	ne := [][2]Value{
		{String("1"), Number(1)},
		{Bool(false), Number(0)},
		{List(Number(1)), List(Number(1), Number(2))},
		{Value{}, String("")},
	}
	for _, p := range ne {
		if p[0].Equal(p[1]) {
			t.Errorf("%v == %v", p[0], p[1])
		}
	}
}

func TestValueString(t *testing.T) {
	cases := map[string]Value{
		"db-07":                String("db-07"),
		"0.25":                 Number(0.25),
		"1e+09":                Number(1e9),
		"true":                 Bool(true),
		"2026-09-25T12:00:00Z": Time(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)),
		"[1, a]":               List(Number(1), String("a")),
		"":                     {},
	}
	for want, v := range cases {
		if got := v.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
		if got := string(v.Append([]byte("x"))); got != "x"+want {
			t.Errorf("Append = %q, want %q", got, "x"+want)
		}
	}
	buf := make([]byte, 0, 64)
	if n := testing.AllocsPerRun(10, func() { buf = Number(0.25).Append(buf[:0]) }); n != 0 {
		t.Errorf("Append of a number allocates %v times", n)
	}
}

// Every attribute of every entity is a Value, so its size is paid hundreds of thousands of times.
func TestValueStaysSmall(t *testing.T) {
	if n := unsafe.Sizeof(Value{}); n > 40 {
		t.Errorf("a Value is %d bytes, want at most 40", n)
	}
}

func TestValueKeepsEachTypesMeaning(t *testing.T) {
	when := time.Date(2026, 9, 26, 1, 2, 3, 4, time.UTC)
	cases := []struct {
		v    Value
		num  float64
		b    bool
		at   time.Time
		list int
	}{
		{v: Number(-2.5), num: -2.5},
		{v: Bool(true), b: true},
		{v: Time(when), at: when},
		{v: List(Number(1), String("x")), list: 2},
		{v: String("s")},
	}
	for _, c := range cases {
		if c.v.Num() != c.num || c.v.Bool() != c.b || !c.v.Time().Equal(c.at) || len(c.v.List()) != c.list {
			t.Errorf("%v: num %v bool %v time %v list %d", c.v, c.v.Num(), c.v.Bool(), c.v.Time(), len(c.v.List()))
		}
	}
	if Number(1).Num() == 0 || Bool(true).Num() != 0 || Time(when).Num() != 0 {
		t.Error("a type's number leaks into another's")
	}
	if !Number(0).Equal(Number(math.Copysign(0, -1))) || Number(math.NaN()).Equal(Number(math.NaN())) {
		t.Error("numbers must compare as floats: 0 equals -0, NaN equals nothing")
	}
}

func TestUnitRidesOnANumber(t *testing.T) {
	v := Number(16 << 30).In(UnitBytes)
	if v.Unit() != UnitBytes || v.Num() != 16<<30 {
		t.Errorf("unit %q num %v", v.Unit(), v.Num())
	}
	if Number(1).Unit() != UnitNone || String("x").In(UnitBytes).Unit() != UnitNone {
		t.Error("a unit on a plain number or a string")
	}
	if Number(1).In("furlongs").Unit() != UnitNone {
		t.Error("an unknown unit was kept")
	}
	if Number(1).In(UnitBytes).Equal(Number(1)) || !Number(1).In(UnitSeconds).Equal(Number(1).In(UnitSeconds)) {
		t.Error("units do not take part in equality")
	}
}

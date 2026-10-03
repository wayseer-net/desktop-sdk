package model

import (
	"math"
	"slices"
	"strconv"
	"time"
)

// Type is the dynamic type held by a Value.
type Type uint8

// Value types.
const (
	TypeNone Type = iota
	TypeString
	TypeNumber
	TypeBool
	TypeTime
	TypeList
)

// Value is a typed attribute value; a tagged struct rather than an interface to keep hot paths cheap.
// It is 40 bytes: one word holds a number, bool or time, and a list is held by pointer. A
// number's unit is an index into knownUnits, in what would be padding.
type Value struct {
	typ  Type
	unit uint8
	n    uint64 // a number's float bits, a bool (0/1) or a time (Unix nanoseconds)
	s    string
	list *[]Value
}

// String makes a string value.
func String(s string) Value { return Value{typ: TypeString, s: s} }

// Number makes a numeric value.
func Number(f float64) Value { return Value{typ: TypeNumber, n: math.Float64bits(f)} }

// In returns v with unit u, if v is a number and u a known unit; otherwise v as it is.
func (v Value) In(u Unit) Value {
	if i := slices.Index(knownUnits, u); v.typ == TypeNumber && i >= 0 {
		v.unit = uint8(i)
	}
	return v
}

// Unit is a number's unit, or none.
func (v Value) Unit() Unit { return knownUnits[v.unit] }

// Bool makes a boolean value.
func Bool(b bool) Value {
	v := Value{typ: TypeBool}
	if b {
		v.n = 1
	}
	return v
}

// Time makes a time value; the instant is kept, the zone is not (PLAN §5.4: UTC inside).
func Time(t time.Time) Value { return Value{typ: TypeTime, n: uint64(t.UnixNano())} }

// List makes a list value.
func List(vs ...Value) Value { return Value{typ: TypeList, list: &vs} }

// Type reports the dynamic type.
func (v Value) Type() Type { return v.typ }

// Str returns the string, or "" for other types.
func (v Value) Str() string { return v.s }

// Num returns the number, or 0 for other types.
func (v Value) Num() float64 {
	if v.typ != TypeNumber {
		return 0
	}
	return math.Float64frombits(v.n)
}

// Bool returns the boolean, or false for other types.
func (v Value) Bool() bool { return v.typ == TypeBool && v.n == 1 }

// Time returns the instant in UTC, or the zero time for other types.
func (v Value) Time() time.Time {
	if v.typ != TypeTime {
		return time.Time{}
	}
	return time.Unix(0, int64(v.n)).UTC()
}

// List returns the elements, or nil for other types; callers must not modify them.
func (v Value) List() []Value {
	if v.list == nil {
		return nil
	}
	return *v.list
}

// Equal reports whether v and w have the same type and value.
func (v Value) Equal(w Value) bool {
	switch {
	case v.typ != w.typ || v.s != w.s || v.unit != w.unit:
		return false
	case v.typ == TypeNumber:
		return v.Num() == w.Num()
	case v.n != w.n:
		return false
	}
	return slices.EqualFunc(v.List(), w.List(), Value.Equal)
}

// String formats v for display and debugging.
func (v Value) String() string {
	if v.typ == TypeString {
		return v.s
	}
	return string(v.Append(nil))
}

// Append appends v as String formats it.
func (v Value) Append(b []byte) []byte {
	switch v.typ {
	case TypeString:
		return append(b, v.s...)
	case TypeNumber:
		return strconv.AppendFloat(b, v.Num(), 'g', -1, 64)
	case TypeBool:
		return strconv.AppendBool(b, v.Bool())
	case TypeTime:
		return v.Time().AppendFormat(b, time.RFC3339Nano)
	case TypeList:
		b = append(b, '[')
		for i, e := range v.List() {
			if i > 0 {
				b = append(b, ", "...)
			}
			b = e.Append(b)
		}
		return append(b, ']')
	}
	return b
}

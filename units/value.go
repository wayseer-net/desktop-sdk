package units

import (
	"strconv"
	"strings"

	"wayseer.dev/sdk/model"
)

// AppendValue appends v for reading: a number through its unit, a list item by item, and any
// other value as model.Value formats it.
func AppendValue(b []byte, v model.Value) []byte {
	switch v.Type() {
	case model.TypeNumber:
		return Append(b, v.Num(), v.Unit())
	case model.TypeList:
		b = append(b, '[')
		for i, e := range v.List() {
			if i > 0 {
				b = append(b, ", "...)
			}
			b = AppendValue(b, e)
		}
		return append(b, ']')
	}
	return v.Append(b)
}

// ValueString is v as AppendValue formats it.
func ValueString(v model.Value) string {
	if v.Type() == model.TypeString {
		return v.Str()
	}
	return string(AppendValue(nil, v))
}

// family is a kind of quantity a suffix may name; units of one family compare.
type family uint8

const (
	anyFamily family = iota
	bytesFamily
	bitsFamily
	secondsFamily
	countFamily
	percentFamily
)

// suffixes are the unit suffixes a quantity may end with, and what each multiplies by.
var suffixes = map[string]struct {
	fam    family
	factor float64
}{
	"B": {bytesFamily, 1}, "KiB": {bytesFamily, 1 << 10}, "MiB": {bytesFamily, 1 << 20},
	"GiB": {bytesFamily, 1 << 30}, "TiB": {bytesFamily, 1 << 40}, "PiB": {bytesFamily, 1 << 50}, "EiB": {bytesFamily, 1 << 60},
	"bit": {bitsFamily, 1}, "kbit": {bitsFamily, 1e3}, "Mbit": {bitsFamily, 1e6},
	"Gbit": {bitsFamily, 1e9}, "Tbit": {bitsFamily, 1e12}, "Pbit": {bitsFamily, 1e15},
	"ns": {secondsFamily, 1e-9}, "µs": {secondsFamily, 1e-6}, "us": {secondsFamily, 1e-6}, "ms": {secondsFamily, 1e-3},
	"s": {secondsFamily, 1}, "min": {secondsFamily, 60}, "h": {secondsFamily, 3600}, "d": {secondsFamily, 86400},
	"k": {countFamily, 1e3}, "M": {countFamily, 1e6}, "G": {countFamily, 1e9}, "T": {countFamily, 1e12},
	"%": {percentFamily, 1},
}

// familyOf is the family of unit u and whether it is a rate; a unit-less number takes any.
func familyOf(u model.Unit) (family, bool) {
	switch u {
	case model.UnitBytes:
		return bytesFamily, false
	case model.UnitBytesPS:
		return bytesFamily, true
	case model.UnitBits:
		return bitsFamily, false
	case model.UnitBitsPS:
		return bitsFamily, true
	case model.UnitSeconds:
		return secondsFamily, false
	case model.UnitCount:
		return countFamily, false
	case model.UnitPerSec:
		return countFamily, true
	case model.UnitPercent, model.UnitRatio:
		return percentFamily, false
	}
	return anyFamily, true
}

// Parse reads s, a number with an optional unit suffix such as "8GiB", "200ms" or "80%", as a
// number in unit u's terms. It fails if s is not a quantity or its suffix is not of u's kind.
func Parse(s string, u model.Unit) (float64, bool) {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n, true
	}
	end := strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789.+-eE", r) })
	if end < 0 {
		end = len(s)
	}
	for ; end > 0; end-- { // back off a trailing "e" or sign that is not part of the number
		if _, err := strconv.ParseFloat(s[:end], 64); err == nil {
			break
		}
	}
	if end == 0 {
		return 0, false
	}
	n, _ := strconv.ParseFloat(s[:end], 64)
	return applySuffix(n, s[end:], u)
}

// applySuffix multiplies n by suffix's factor, if suffix is of unit u's family.
func applySuffix(n float64, suffix string, u model.Unit) (float64, bool) {
	fam, rate := familyOf(u)
	perSec := strings.HasSuffix(suffix, "/s") && suffix != "/s"
	if perSec {
		suffix = strings.TrimSuffix(suffix, "/s")
	}
	sf, ok := suffixes[suffix]
	switch {
	case !ok, perSec && !rate, fam != anyFamily && sf.fam != fam:
		return 0, false
	case sf.fam == percentFamily && u == model.UnitRatio:
		return n / 100, true
	}
	return n * sf.factor, true
}

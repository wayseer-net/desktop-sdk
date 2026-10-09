// Package units formats values with their units for reading: three significant digits, scaled
// to a prefix, appended to a buffer without allocating.
package units

import (
	"math"
	"strconv"

	"wayseer.dev/sdk/model"
)

// Beyond these magnitudes a number shows in scientific notation.
const (
	sciAbove = 1e15
	sciBelow = 1e-6
	sciTop   = 1e6 // for a value past its unit's largest prefix
)

// prefix is one step of a unit's scale: values from factor up show divided by it.
type prefix struct {
	factor float64
	suffix string
}

// scale is a unit's prefixes, smallest first, and the index of the one with factor 1.
type scale struct {
	prefixes []prefix
	base     int
}

var (
	si    = scale{prefixes: []prefix{{1, ""}, {1e3, "k"}, {1e6, "M"}, {1e9, "G"}, {1e12, "T"}, {1e15, "P"}, {1e18, "E"}}}
	bits  = scale{prefixes: []prefix{{1, " bit"}, {1e3, " kbit"}, {1e6, " Mbit"}, {1e9, " Gbit"}, {1e12, " Tbit"}, {1e15, " Pbit"}}}
	bytes = scale{prefixes: []prefix{{1, " B"}, {1 << 10, " KiB"}, {1 << 20, " MiB"}, {1 << 30, " GiB"}, {1 << 40, " TiB"}, {1 << 50, " PiB"}, {1 << 60, " EiB"}}}
	secs  = scale{prefixes: []prefix{{1e-9, " ns"}, {1e-6, " µs"}, {1e-3, " ms"}, {1, " s"}, {60, " min"}, {3600, " h"}, {86400, " d"}}, base: 3}

	watts     = scale{prefixes: []prefix{{1e-3, " mW"}, {1, " W"}, {1e3, " kW"}, {1e6, " MW"}, {1e9, " GW"}}, base: 1}
	wattHours = scale{prefixes: []prefix{{1, " Wh"}, {1e3, " kWh"}, {1e6, " MWh"}, {1e9, " GWh"}, {1e12, " TWh"}}}
	volts     = scale{prefixes: []prefix{{1e-3, " mV"}, {1, " V"}, {1e3, " kV"}}, base: 1}
	amperes   = scale{prefixes: []prefix{{1e-6, " µA"}, {1e-3, " mA"}, {1, " A"}, {1e3, " kA"}}, base: 2}
	hertz     = scale{prefixes: []prefix{{1, " Hz"}, {1e3, " kHz"}, {1e6, " MHz"}, {1e9, " GHz"}}}
	lux       = scale{prefixes: []prefix{{1, " lx"}, {1e3, " klx"}}}
	pascals   = scale{prefixes: []prefix{{1, " Pa"}, {100, " hPa"}}} // weather reads in hPa, four digits whole
)

// unscaled are the units shown as they are, with a suffix.
var unscaled = map[model.Unit]string{
	model.UnitCelsius: " °C", model.UnitPPM: " ppm", model.UnitMicrogramsPerM3: " µg/m³", model.UnitDBm: " dBm",
}

// scales are the units shown under a prefix.
var scales = map[model.Unit]*scale{
	model.UnitWatts: &watts, model.UnitWattHours: &wattHours, model.UnitVolts: &volts, model.UnitAmperes: &amperes,
	model.UnitHertz: &hertz, model.UnitLux: &lux, model.UnitPascals: &pascals,
}

// Append appends v with unit u, to three significant digits: "93.2%", "1.5 KiB", "212 ms",
// "12.3k/s". A value without a unit is never scaled, so ids and ports read as themselves.
func Append(b []byte, v float64, u model.Unit) []byte {
	switch {
	case math.IsNaN(v):
		return append(b, "–"...)
	case math.IsInf(v, 1):
		return append(b, "∞"...)
	case math.IsInf(v, -1):
		return append(b, "-∞"...)
	case v < 0:
		b = append(b, '-')
	}
	a := math.Abs(v)
	switch u {
	case model.UnitRatio:
		return append(appendSig(b, a*100), '%')
	case model.UnitPercent:
		return append(appendSig(b, a), '%')
	case model.UnitSeconds:
		return appendScaled(b, a, &secs, "")
	case model.UnitBytes:
		return appendScaled(b, a, &bytes, "")
	case model.UnitBytesPS:
		return appendScaled(b, a, &bytes, "/s")
	case model.UnitBits:
		return appendScaled(b, a, &bits, "")
	case model.UnitBitsPS:
		return appendScaled(b, a, &bits, "/s")
	case model.UnitCount:
		return appendScaled(b, a, &si, "")
	case model.UnitPerSec:
		return appendScaled(b, a, &si, "/s")
	}
	if s, ok := scales[u]; ok {
		return appendScaled(b, a, s, "")
	}
	return append(appendSig(b, a), unscaled[u]...)
}

// appendScaled appends a ≥ 0 under the largest prefix it reaches, moving up one when rounding
// carries it there, so the mantissa stays under 1000.
func appendScaled(b []byte, a float64, s *scale, tail string) []byte {
	ps := s.prefixes
	i, r := s.shown(a)
	if r >= sciTop {
		return append(append(appendSci(b, a), ps[s.base].suffix...), tail...)
	}
	return append(append(appendSig(b, r), ps[i].suffix...), tail...)
}

// shown is the index of the prefix a ≥ 0 shows under and its mantissa, rounded.
func (s *scale) shown(a float64) (int, float64) {
	ps := s.prefixes
	i := s.reached(a)
	r, _ := round3(a / ps[i].factor)
	if i+1 < len(ps) && (r >= 1000 || r*ps[i].factor >= ps[i+1].factor) {
		r, i = r*ps[i].factor/ps[i+1].factor, i+1
	}
	return i, r
}

// Step is how much one in the last digit Append shows for v is worth, in u; 0 for zero, NaN
// and infinities, which have no last digit.
func Step(v float64, u model.Unit) float64 {
	a := math.Abs(v)
	if a == 0 || math.IsNaN(a) || math.IsInf(a, 0) {
		return 0
	}
	switch u {
	case model.UnitRatio:
		return sigStep(a*100) / 100
	case model.UnitSeconds:
		return scaledStep(a, &secs)
	case model.UnitBytes, model.UnitBytesPS:
		return scaledStep(a, &bytes)
	case model.UnitBits, model.UnitBitsPS:
		return scaledStep(a, &bits)
	case model.UnitCount, model.UnitPerSec:
		return scaledStep(a, &si)
	}
	if s, ok := scales[u]; ok {
		return scaledStep(a, s)
	}
	return sigStep(a)
}

// scaledStep is Step for a > 0 shown under s's prefixes.
func scaledStep(a float64, s *scale) float64 {
	i, r := s.shown(a)
	if r >= sciTop {
		return sigStep(a)
	}
	return sigStep(r) * s.prefixes[i].factor
}

// sigStep is the worth of the last digit appendSig shows for a > 0.
func sigStep(a float64) float64 {
	if a >= sciAbove || a < sciBelow {
		return math.Pow10(int(math.Floor(math.Log10(a))) - 2)
	}
	_, d := round3(a)
	return math.Pow10(-d)
}

// reached is the index of the largest prefix a ≥ 0 reaches; zero shows under the base.
func (s *scale) reached(a float64) int {
	if a == 0 {
		return s.base
	}
	i := 0
	for i+1 < len(s.prefixes) && a >= s.prefixes[i+1].factor {
		i++
	}
	return i
}

// appendSig appends a ≥ 0 to three significant digits, but no fewer than its whole digits,
// without trailing zeros.
func appendSig(b []byte, a float64) []byte {
	if a != 0 && (a >= sciAbove || a < sciBelow) {
		return appendSci(b, a)
	}
	r, d := round3(a)
	return appendFixed(b, r, d)
}

// appendSci appends a > 0 as a mantissa of three significant digits and a power of ten.
func appendSci(b []byte, a float64) []byte {
	e := int(math.Floor(math.Log10(a)))
	m := math.Round(a/math.Pow10(e)*100) / 100
	if m >= 10 {
		m, e = m/10, e+1
	}
	return strconv.AppendInt(append(appendFixed(b, m, 2), 'e'), int64(e), 10)
}

// round3 rounds a ≥ 0 to three significant digits, returning it and its decimal places.
func round3(a float64) (float64, int) {
	d := 2
	for x := a; x >= 10 && d > 0; x /= 10 {
		d--
	}
	for x := a; x > 0 && x < 1; x *= 10 {
		d++
	}
	p := math.Pow10(d)
	return math.Round(a*p) / p, d
}

// appendFixed appends r with at most d decimal places, dropping trailing zeros.
func appendFixed(b []byte, r float64, d int) []byte {
	b = strconv.AppendFloat(b, r, 'f', d, 64)
	if d == 0 {
		return b
	}
	for b[len(b)-1] == '0' {
		b = b[:len(b)-1]
	}
	if b[len(b)-1] == '.' {
		b = b[:len(b)-1]
	}
	return b
}

// AppendCount appends the whole number n with commas between groups of three digits, e.g. "12,345".
func AppendCount(b []byte, n int) []byte {
	if n < 0 {
		b, n = append(b, '-'), -n
	}
	s := strconv.Itoa(n)
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, s[i])
	}
	return b
}

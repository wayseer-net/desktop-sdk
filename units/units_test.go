package units

import (
	"math"
	"testing"
	"wayseer/pkg/sdk/model"
)

var cases = []struct {
	v    float64
	unit model.Unit
	want string
}{
	// Plain numbers keep their whole digits and are never scaled: a port stays a port.
	{0, model.UnitNone, "0"},
	{8080, model.UnitNone, "8080"},
	{12345.6, model.UnitNone, "12346"},
	{0.333333, model.UnitNone, "0.333"},
	{0.000123456, model.UnitNone, "0.000123"},
	{-2.5, model.UnitNone, "-2.5"},
	{math.Copysign(0, -1), model.UnitNone, "0"},
	{-0.0001, model.UnitPercent, "-0.0001%"},
	{1.5e-9, model.UnitNone, "1.5e-9"},
	{6.02e23, model.UnitNone, "6.02e23"},
	{9.996, model.UnitNone, "10"},

	{12, model.UnitCount, "12"},
	{999, model.UnitCount, "999"},
	{999.7, model.UnitCount, "1k"},
	{12345.6, model.UnitCount, "12.3k"},
	{-4.2e6, model.UnitCount, "-4.2M"},
	{3e30, model.UnitCount, "3e30"},
	{412.3, model.UnitPerSec, "412/s"},
	{3.25, model.UnitPerSec, "3.25/s"},
	{0.25, model.UnitPerSec, "0.25/s"},
	{15300, model.UnitPerSec, "15.3k/s"},

	{0, model.UnitBytes, "0 B"},
	{512, model.UnitBytes, "512 B"},
	{1000, model.UnitBytes, "0.977 KiB"},
	{1023.9, model.UnitBytes, "1 KiB"},
	{1536, model.UnitBytes, "1.5 KiB"},
	{16 << 30, model.UnitBytes, "16 GiB"},
	{-1536, model.UnitBytes, "-1.5 KiB"},
	{3 << 30, model.UnitBytesPS, "3 GiB/s"},
	{1 << 60, model.UnitBytes, "1 EiB"},
	{1e9, model.UnitBits, "1 Gbit"},
	{2.5e6, model.UnitBitsPS, "2.5 Mbit/s"},
	{800, model.UnitBitsPS, "800 bit/s"},

	{0, model.UnitSeconds, "0 s"},
	{4.2e-8, model.UnitSeconds, "42 ns"},
	{0.00042, model.UnitSeconds, "420 µs"},
	{0.2123, model.UnitSeconds, "212 ms"},
	{0.9996, model.UnitSeconds, "1 s"},
	{1.843, model.UnitSeconds, "1.84 s"},
	{59.96, model.UnitSeconds, "1 min"},
	{150, model.UnitSeconds, "2.5 min"},
	{5400, model.UnitSeconds, "1.5 h"},
	{3 * 86400, model.UnitSeconds, "3 d"},
	{400 * 86400, model.UnitSeconds, "400 d"},
	{-0.2, model.UnitSeconds, "-200 ms"},

	{93.24, model.UnitPercent, "93.2%"},
	{100, model.UnitPercent, "100%"},
	{5.123, model.UnitPercent, "5.12%"},
	{0.932, model.UnitRatio, "93.2%"},
	{1, model.UnitRatio, "100%"},

	{math.NaN(), model.UnitPercent, "–"},
	{math.Inf(1), model.UnitBytes, "∞"},
	{math.Inf(-1), model.UnitNone, "-∞"},
}

func TestAppendFormatsValuesWithTheirUnits(t *testing.T) {
	for _, c := range cases {
		if got := string(Append(nil, c.v, c.unit)); got != c.want {
			t.Errorf("Append(%v, %q) = %q, want %q", c.v, c.unit, got, c.want)
		}
	}
}

func TestAppendKeepsWhatIsAlreadyInTheBuffer(t *testing.T) {
	if got := string(Append([]byte("p95 "), 0.2123, model.UnitSeconds)); got != "p95 212 ms" {
		t.Errorf("got %q", got)
	}
	if got := string(Append([]byte("x"), math.Copysign(0, -1), model.UnitNone)); got != "x0" {
		t.Errorf("negative zero after a prefix: %q", got)
	}
}

func TestAppendDoesNotAllocate(t *testing.T) {
	b := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() {
		for _, c := range cases {
			b = Append(b[:0], c.v, c.unit)
		}
	})
	if allocs != 0 {
		t.Errorf("Append allocates %v times per run", allocs)
	}
}

func TestAppendCountGroupsThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 200_000: "200,000", 1_234_567: "1,234,567", -4321: "-4,321"} {
		if got := string(AppendCount(nil, n)); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
}

func TestUnitValueFormatsThroughItsUnit(t *testing.T) {
	cases := map[string]model.Value{
		"16 GiB":  model.Number(16 << 30).In(model.UnitBytes),
		"212 ms":  model.Number(0.2123).In(model.UnitSeconds),
		"8080":    model.Number(8080),
		"db-07":   model.String("db-07"),
		"[1 KiB]": model.List(model.Number(1024).In(model.UnitBytes)),
		"true":    model.Bool(true),
	}
	for want, v := range cases {
		if got := string(AppendValue(nil, v)); got != want {
			t.Errorf("AppendValue(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestUnitQuantitiesParse(t *testing.T) {
	cases := []struct {
		s    string
		unit model.Unit
		want float64
		ok   bool
	}{
		{"8", model.UnitBytes, 8, true},
		{"8GiB", model.UnitBytes, 8 << 30, true},
		{"1.5KiB", model.UnitBytes, 1536, true},
		{"8GiB", model.UnitNone, 8 << 30, true},
		{"200ms", model.UnitSeconds, 0.2, true},
		{"2h", model.UnitSeconds, 7200, true},
		{"80%", model.UnitPercent, 80, true},
		{"80%", model.UnitRatio, 0.8, true},
		{"12k", model.UnitCount, 12000, true},
		{"3Gbit/s", model.UnitBitsPS, 3e9, true},
		{"8GiB", model.UnitSeconds, 0, false},
		{"8 GiB", model.UnitBytes, 0, false},
		{"8furlongs", model.UnitBytes, 0, false},
		{"web-01", model.UnitNone, 0, false},
	}
	for _, c := range cases {
		got, ok := Parse(c.s, c.unit)
		if ok != c.ok || ok && math.Abs(got-c.want) > 1e-9*math.Abs(c.want) {
			t.Errorf("Parse(%q, %q) = %v %v, want %v %v", c.s, c.unit, got, ok, c.want, c.ok)
		}
	}
}

func TestStepIsTheLastPlaceAppendShows(t *testing.T) {
	for _, c := range []struct {
		v    float64
		unit model.Unit
		want float64
	}{
		{0.123, model.UnitRatio, 0.001},   // 12.3%
		{93.2, model.UnitPercent, 0.1},    // 93.2%
		{1536, model.UnitBytes, 10.24},    // 1.5 KiB, to 1.50
		{0.288, model.UnitSeconds, 0.001}, // 288 ms
		{-3, model.UnitCount, 0.01},
		{12345.6, model.UnitNone, 1},
		{999.9, model.UnitCount, 10}, // 1k, to 1.00k
		{2e20, model.UnitNone, 1e18},
		{0, model.UnitNone, 0},
		{math.NaN(), model.UnitRatio, 0},
		{math.Inf(1), model.UnitBytes, 0},
	} {
		if got := Step(c.v, c.unit); math.Abs(got-c.want) > c.want*1e-9 {
			t.Errorf("Step(%v, %q) = %v, want %v", c.v, c.unit, got, c.want)
		}
	}
}

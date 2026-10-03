package wire

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/data"
	pb "wayseer/pkg/sdk/proto/modulev1"

	"go.yaml.in/yaml/v3"
)

// changeSet is a random change set whose empty lists and maps are nil, as decoding makes them.
type changeSet struct{ *sdk.ChangeSet }

func (changeSet) Generate(r *rand.Rand, size int) reflect.Value {
	var cs sdk.ChangeSet
	for range r.Intn(size + 1) {
		cs.Removes = append(cs.Removes, randomRef(r))
	}
	for range r.Intn(size + 1) {
		cs.RemoveEdges = append(cs.RemoveEdges, sdk.EdgeKey{From: randomRef(r), To: randomRef(r), Rel: sdk.RelTalksTo})
	}
	for range r.Intn(size + 1) {
		cs.Upserts = append(cs.Upserts, randomEntity(r))
	}
	for range r.Intn(size + 1) {
		cs.Edges = append(cs.Edges, sdk.Edge{From: randomRef(r), To: randomRef(r), Rel: sdk.RelDependsOn, Weight: r.NormFloat64(), Attrs: randomAttrs(r, 2), Source: "m", Traffic: randomTraffic(r)})
	}
	for range r.Intn(size + 1) {
		cs.Events = append(cs.Events, randomEvent(r))
	}
	return reflect.ValueOf(changeSet{&cs})
}

func randomRef(r *rand.Rand) sdk.EntityRef {
	ref, _ := sdk.NewEntityRef("m", sdk.KindHost, fmt.Sprintf("h%d", r.Intn(1000)))
	return ref
}

func randomTraffic(r *rand.Rand) sdk.Traffic {
	units := []sdk.TrafficUnit{sdk.TrafficNone, sdk.TrafficRequests, sdk.TrafficBytes, sdk.TrafficMessages}
	u := units[r.Intn(len(units))]
	if u == sdk.TrafficNone {
		return sdk.Traffic{}
	}
	return sdk.Traffic{Rate: r.ExpFloat64() * 1e3, Unit: u}
}

func randomTime(r *rand.Rand) time.Time {
	if r.Intn(5) == 0 {
		return time.Time{}
	}
	return time.Unix(0, r.Int63()-r.Int63()).UTC()
}

func randomEntity(r *rand.Rand) sdk.Entity {
	e := sdk.Entity{
		Ref: randomRef(r), Kind: sdk.KindHost, Name: fmt.Sprint(r.Int()), Attrs: randomAttrs(r, 4),
		Status: sdk.Status{Level: sdk.StatusLevel(r.Intn(5)), Reason: fmt.Sprint(r.Int())},
		Source: "m", Seen: randomTime(r),
	}
	if r.Intn(2) == 0 {
		e.Place = sdk.At(float32(r.Float64()*180-90), float32(r.Float64()*360-180))
	}
	for range r.Intn(3) {
		e.Tags = append(e.Tags, fmt.Sprint(r.Intn(10)))
	}
	return e
}

func randomEvent(r *rand.Rand) sdk.Event {
	e := sdk.Event{
		ID: fmt.Sprint(r.Int()), At: randomTime(r), Severity: sdk.Severity(r.Intn(5)),
		Kind: "log", Message: "m\x00é", Fields: randomAttrs(r, 3), Source: "m",
	}
	if r.Intn(2) == 0 {
		e.Entity = randomRef(r)
	}
	return e
}

func randomAttrs(r *rand.Rand, n int) map[string]sdk.Value {
	k := r.Intn(n + 1)
	if k == 0 {
		return nil
	}
	m := make(map[string]sdk.Value, k)
	for i := range k {
		m[fmt.Sprint("a", i)] = randomValue(r, 2)
	}
	return m
}

func randomValue(r *rand.Rand, depth int) sdk.Value {
	switch r.Intn(6) {
	case 1:
		return sdk.String(fmt.Sprint(r.Int()))
	case 2:
		return sdk.Number(r.NormFloat64() * 1e9).In([]sdk.Unit{sdk.UnitNone, sdk.UnitBytes, sdk.UnitSeconds}[r.Intn(3)])
	case 3:
		return sdk.Bool(r.Intn(2) == 0)
	case 4:
		return sdk.Time(randomTime(r))
	case 5:
		if depth > 0 {
			var vs []sdk.Value
			for range r.Intn(3) {
				vs = append(vs, randomValue(r, depth-1))
			}
			return sdk.List(vs...)
		}
	}
	return sdk.Value{}
}

func TestChangeSetsRoundTrip(t *testing.T) {
	f := func(c changeSet) bool {
		got := DecodeChangeSet(EncodeChangeSet(c.ChangeSet))
		return reflect.DeepEqual(got, c.ChangeSet)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestAnEdgeFromAnOlderModuleCarriesNoTraffic(t *testing.T) {
	cs := DecodeChangeSet(&pb.ChangeSet{Edges: []*pb.Edge{{From: "m/host/a", To: "m/host/b", Rel: "talks_to", Source: "m"}}})
	if got := cs.Edges[0].Traffic; got != (sdk.Traffic{}) {
		t.Errorf("traffic %+v; want none", got)
	}
}

func TestAnEntityFromAnOlderModuleHasNoPlace(t *testing.T) {
	cs := DecodeChangeSet(&pb.ChangeSet{Upserts: []*pb.Entity{{Ref: "m/host/a", Kind: "host", Source: "m"}}})
	if got := cs.Upserts[0].Place; got != (sdk.Place{}) {
		t.Errorf("place %+v; want none", got)
	}
}

func TestAPlaceOffEarthIsRefused(t *testing.T) {
	e := &pb.Entity{Ref: "m/host/a", Kind: "host", Source: "m", Place: &pb.Place{Lat: 95, Lon: 0}}
	got := DecodeChangeSet(&pb.ChangeSet{Upserts: []*pb.Entity{e}}).Upserts[0]
	if err := got.Validate(); err == nil {
		t.Error("a latitude of 95 validated")
	}
}

func TestTrafficInAnUnknownUnitIsRefused(t *testing.T) {
	e := &pb.Edge{From: "m/host/a", To: "m/host/b", Rel: "talks_to", Source: "m", Traffic: &pb.Traffic{Rate: 1, Unit: "packets"}}
	if err := DecodeChangeSet(&pb.ChangeSet{Edges: []*pb.Edge{e}}).Edges[0].Validate(); err == nil {
		t.Error("an unknown unit validated")
	}
}

func TestAnEmptyChangeSetRoundTrips(t *testing.T) {
	for _, cs := range []*sdk.ChangeSet{{}, nil} {
		if got := DecodeChangeSet(EncodeChangeSet(cs)); !reflect.DeepEqual(got, &sdk.ChangeSet{}) {
			t.Errorf("%+v became %+v", cs, got)
		}
	}
}

func TestLocalTimesArriveInUTC(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 5, time.FixedZone("x", 3600))
	cs := &sdk.ChangeSet{Events: []sdk.Event{{At: at, Fields: map[string]sdk.Value{"t": sdk.Time(at)}}}}
	got := DecodeChangeSet(EncodeChangeSet(cs)).Events[0]
	if !got.At.Equal(at) || got.At.Location() != time.UTC || !got.Fields["t"].Equal(sdk.Time(at)) {
		t.Errorf("at %v, field %v", got.At, got.Fields["t"])
	}
}

func TestHealthRoundTrips(t *testing.T) {
	for _, h := range []sdk.Health{{}, {Disconnected: true}, {Err: errors.New("403 from api"), Note: "no nodes"}, {Err: errors.New("")}} {
		got := DecodeHealth(EncodeHealth(h))
		if got.Disconnected != h.Disconnected || got.Note != h.Note || (got.Err == nil) != (h.Err == nil) ||
			(h.Err != nil && got.Err.Error() != h.Err.Error()) {
			t.Errorf("%+v became %+v", h, got)
		}
	}
}

func TestInfoRoundTrips(t *testing.T) {
	info := sdk.Info{Kind: "file", Version: "1.2.3", Description: "reads a file"}
	caps := []string{"discover", "series"}
	got, gotCaps, v := DecodeInfo(EncodeInfo(info, caps))
	if got != info || !reflect.DeepEqual(gotCaps, caps) || v != Protocol {
		t.Errorf("%+v %v %v", got, gotCaps, v)
	}
}

type options struct {
	URL   string            `yaml:"url"`
	Every time.Duration     `yaml:"every"`
	Tags  []string          `yaml:"tags"`
	Extra map[string]string `yaml:"extra"`
}

const optionsYAML = `modules:
  - type: file
    options: &opts
      url: "http://example.test/x"
      every: 30s
      tags: [a, 'b', "c"]
      extra:
        base: *opts_url
`

func TestConfigRoundTripsWithItsLines(t *testing.T) {
	src := "anchors:\n  u: &opts_url http://example.test\n" + optionsYAML
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	opts := doc.Content[0].Content[3].Content[0].Content[3]
	cfg := sdk.Config{Name: "files", Line: 4, Options: *opts}
	got := DecodeConfig(EncodeConfig(cfg))
	if got.Name != cfg.Name || got.Line != cfg.Line || !reflect.DeepEqual(got.Options, cfg.Options) {
		t.Fatalf("config %+v\nbecame %+v", cfg, got)
	}
	var want, have options
	if err := cfg.Decode(&want); err != nil {
		t.Fatal(err)
	}
	if err := got.Decode(&have); err != nil || !reflect.DeepEqual(have, want) || have.Extra["base"] != "http://example.test" {
		t.Errorf("decoded %+v, %v; want %+v", have, err, want)
	}

	var strict struct{ URL string }
	errWant, errGot := cfg.Decode(&strict), got.Decode(&strict)
	if errWant == nil || errGot == nil || errGot.Error() != errWant.Error() {
		t.Errorf("unknown-option error %v, want %v", errGot, errWant)
	}
}

func TestAConfigWithoutOptionsRoundTrips(t *testing.T) {
	got := DecodeConfig(EncodeConfig(sdk.Config{Name: "x", Line: 2}))
	if !reflect.DeepEqual(got, sdk.Config{Name: "x", Line: 2}) {
		t.Errorf("%+v", got)
	}
}

func TestMetricsRoundTrip(t *testing.T) {
	ms := []sdk.Metric{
		{Name: "cpu.utilisation", Unit: sdk.UnitPercent, Description: "CPU", Kinds: []sdk.Kind{sdk.KindHost, sdk.KindPod}, Native: "rate(x[1m])", Extra: true, Joined: true},
		{Name: "up"},
	}
	if got := DecodeMetrics(EncodeMetrics(ms)); !reflect.DeepEqual(got, ms) {
		t.Errorf("%+v", got)
	}
}

func TestSeriesQueriesRoundTrip(t *testing.T) {
	f, err := data.ParseFilter(`kind:host,pod source:a status:warn metric:cpu same_as:m/host/h1 #prod has:ip name~web cpu>=50`)
	if err != nil {
		t.Fatal(err)
	}
	q := sdk.SeriesQuery{
		Entities: []sdk.EntityRef{"m/host/h1"}, Filter: f, Metrics: []string{"cpu.utilisation"},
		Window: sdk.TimeWindow{From: time.Unix(10, 0).UTC(), To: time.Unix(20, 0).UTC()},
		Step:   15 * time.Second, Agg: sdk.AggP95, Native: "up", Top: 500,
	}
	if got := DecodeSeriesQuery(EncodeSeriesQuery(q)); !reflect.DeepEqual(got, q) {
		t.Errorf("%+v\nbecame %+v", q, got)
	}
	if got := DecodeSeriesQuery(EncodeSeriesQuery(sdk.SeriesQuery{})); !reflect.DeepEqual(got, sdk.SeriesQuery{}) {
		t.Errorf("the zero query became %+v", got)
	}
}

func TestSeriesRoundTrip(t *testing.T) {
	ss := []sdk.Series{
		{Ref: sdk.SeriesRef{Entity: "m/host/h1", Metric: "cpu"}, Unit: sdk.UnitPercent, Points: []sdk.Point{{T: 1, V: 2}, {T: 3, V: -4}}},
		{Ref: sdk.SeriesRef{Entity: "m/host/h2", Metric: "cpu"}},
	}
	got, err := DecodeSeries(EncodeSeries(ss))
	if err != nil || !reflect.DeepEqual(got, ss) {
		t.Errorf("%+v, %v", got, err)
	}
}

func TestSeriesWithUnevenColumnsIsRefused(t *testing.T) {
	pb := EncodeSeries([]sdk.Series{{Points: []sdk.Point{{T: 1, V: 2}}}})
	pb[0].Values = nil
	if _, err := DecodeSeries(pb); err == nil {
		t.Error("decoded a series with a time and no value")
	}
}

func TestEventQueriesRoundTrip(t *testing.T) {
	q := sdk.EventQuery{
		Entities: []sdk.EntityRef{"m/host/h1"}, Window: sdk.TimeWindow{To: time.Unix(5, 0).UTC()},
		MinSeverity: sdk.SevWarn, Kinds: []string{"deploy"}, Limit: 20,
	}
	if got := DecodeEventQuery(EncodeEventQuery(q)); !reflect.DeepEqual(got, q) {
		t.Errorf("%+v", got)
	}
}

func TestEventsRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	evs := []sdk.Event{randomEvent(r), randomEvent(r)}
	if got := DecodeEvents(EncodeEvents(evs)); !reflect.DeepEqual(got, evs) {
		t.Errorf("%+v", got)
	}
}

func TestRefsRoundTrip(t *testing.T) {
	refs := []sdk.EntityRef{"m/host/a", "m/host/b"}
	if got := DecodeRefs(EncodeRefs(refs)); !reflect.DeepEqual(got, refs) {
		t.Errorf("%v", got)
	}
}

// A 1.0 module sends numbers without units, and a newer one may name a unit this app lacks.
func TestUnitIsOptionalOnTheWire(t *testing.T) {
	for unit, want := range map[string]sdk.Unit{"": sdk.UnitNone, "bytes": sdk.UnitBytes, "furlongs": sdk.UnitNone} {
		v := decodeValue(&pb.Value{Value: &pb.Value_NumberValue{NumberValue: 2}, Unit: unit})
		if v.Num() != 2 || v.Unit() != want {
			t.Errorf("unit %q decodes as %v in %q, want 2 in %q", unit, v.Num(), v.Unit(), want)
		}
	}
}

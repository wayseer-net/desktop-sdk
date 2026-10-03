package data

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"wayseer.dev/sdk/model"
)

func TestParseFilter(t *testing.T) {
	cases := []struct {
		src  string
		want Filter
	}{
		{"", Filter{}},
		{"kind:host", Filter{Kinds: []model.Kind{"host"}}},
		{"kind:host,node  kind:pod", Filter{Kinds: []model.Kind{"host", "node", "pod"}}},
		{"#prod #\"team a\"", Filter{Tags: []string{"prod", "team a"}}},
		{"env=prod cpu>=0.5 name~db has:ip", Filter{Attrs: []Predicate{
			{Key: "env", Op: OpEq, Value: "prod"},
			{Key: "cpu", Op: OpGE, Value: "0.5"},
			{Key: "name", Op: OpContains, Value: "db"},
			{Key: "ip", Op: OpHas},
		}}},
		{`zone!="us east" x<3 y>2 z<=1`, Filter{Attrs: []Predicate{
			{Key: "zone", Op: OpNE, Value: "us east"},
			{Key: "x", Op: OpLT, Value: "3"},
			{Key: "y", Op: OpGT, Value: "2"},
			{Key: "z", Op: OpLE, Value: "1"},
		}}},
		{"source:demo,prom-lan", Filter{Sources: []model.ModuleID{"demo", "prom-lan"}}},
		{"status:warn", Filter{Statuses: []model.StatusLevel{model.StatusWarn}}},
		{"status:crit,warn status:down", Filter{Statuses: []model.StatusLevel{model.StatusCrit, model.StatusWarn, model.StatusDown}}},
		{"metric:cpu.utilisation,http.latency.p95", Filter{Metrics: []string{"cpu.utilisation", "http.latency.p95"}}},
		{`same_as:prom/host/db-07 same_as:"k8s/node/a b"`, Filter{SameAs: []model.EntityRef{"prom/host/db-07", "k8s/node/a b"}}},
		{"cloud.instance_id=i-1 a=b=c", Filter{Attrs: []Predicate{
			{Key: "cloud.instance_id", Op: OpEq, Value: "i-1"}, {Key: "a", Op: OpEq, Value: "b=c"},
		}}},
	}
	for _, c := range cases {
		got, err := ParseFilter(c.src)
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q = %+v, want %+v", c.src, got, c.want)
		}
	}
}

func TestParseFilterErrorsNamePosition(t *testing.T) {
	cases := map[string]string{
		"kind:":           "column 1",
		"kind:Host":       "column 1",
		"env=prod foo":    "column 10",
		"#":               "column 1",
		`a="open`:         "column 1",
		"=x":              "column 1",
		`a="x"y`:          "column 1",
		"has:":            "column 1",
		"kind:a,,b":       "column 1",
		"status:":         "column 1",
		"x=1 status:bad":  "column 5",
		"metric:":         "column 1",
		"metric:a,,b":     "column 1",
		"metric:a b=c":    "", // two terms, both good
		"same_as:":        "column 1",
		"same_as:db-07":   "column 1",
		"same_as:a/b/%zz": "column 1",
	}
	for src, want := range cases {
		if _, err := ParseFilter(src); want == "" {
			if err != nil {
				t.Errorf("%q: %v", src, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %s", src, err, want)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	e := &model.Entity{
		Kind: model.KindHost, Name: "db-07", Tags: []string{"prod", "team a"}, Status: model.Status{Level: model.StatusWarn}, Source: "demo",
		Attrs: map[string]model.Value{
			"env": model.String("prod"), "cpu": model.Number(0.75), "cores": model.Number(8), "load1": model.Number(4.5),
			"ip": model.List(model.String("10.0.0.5")),
		},
	}
	cases := map[string]bool{
		"":                    true,
		"kind:host":           true,
		"kind:pod,host":       true,
		"kind:pod":            false,
		"#prod":               true,
		`#prod #"team a"`:     true,
		"#dev":                false,
		"env=prod":            true,
		"env!=prod":           false,
		"missing!=x":          true,
		"missing=x":           false,
		"cpu>0.5":             true,
		"cpu>=0.75 cpu<=0.75": true,
		"cpu<0.5":             false,
		"cores>10":            false, // numeric, not lexical
		"cores>10.5 ":         false,
		"name~db":             true,
		"name=db-07":          true,
		"name~DB":             true, // contains is case-insensitive
		"has:ip":              true,
		"has:disk":            false,
		"ip~10.0":             true,
		"env>a":               true, // strings compare lexically
		"kind:host env=dev":   false,
		"source:demo":         true,
		"source:prom,demo":    true,
		"source:prom":         false,
		"status:warn":         true, // at or worse than warn
		"status:crit":         false,
		"status:crit,warn":    true,
		"status:ok":           true,
		"load1>4":             true,
		"load1>9":             false,
		"load5>4":             false, // missing
	}
	for src, want := range cases {
		f, err := ParseFilter(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got := f.Match(e); got != want {
			t.Errorf("%q matched %v, want %v", src, got, want)
		}
	}
}

func TestFilterStringRoundTrips(t *testing.T) {
	for _, src := range []string{"", "kind:host,node #prod env=prod", `zone!="us east" #"a b" has:x cpu>=1`, "status:unknown,down kind:pod source:a,b", "metric:cpu.utilisation,disk.used kind:host", `same_as:"a/host/x y" same_as:b/node/z`} {
		f, err := ParseFilter(src)
		if err != nil {
			t.Fatal(err)
		}
		if g, err := ParseFilter(f.String()); err != nil || !reflect.DeepEqual(g, f) || g.IsZero() != (src == "") {
			t.Errorf("%q -> %q -> %+v, %v", src, f.String(), g, err)
		}
	}
}

func FuzzParseFilter(f *testing.F) {
	for _, s := range []string{"kind:host #prod env=prod", `a!="x y" has:b c~d`, "e<=1 f>2", `#"\xff"`, "kind:a/b", "status:warn,crit", "status:ok x=1", "source:demo", "metric:cpu,mem", "same_as:a/host/b", `same_as:"a/host/b c"`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		flt, err := ParseFilter(src)
		if err != nil {
			return
		}
		again, err := ParseFilter(flt.String())
		if err != nil || !reflect.DeepEqual(again, flt) {
			t.Fatalf("%q -> %q -> %+v, %v; want %+v", src, flt.String(), again, err, flt)
		}
		flt.Offers = func(*model.Entity, string) bool { return true }
		flt.Match(&model.Entity{Name: src, Status: model.Status{Level: model.StatusCrit}, Attrs: map[string]model.Value{"a": model.String(src)}})
	})
}

func TestFilterNaNMatchesOnlyNotEqual(t *testing.T) {
	e := &model.Entity{Attrs: map[string]model.Value{"x": model.Number(math.NaN())}}
	for src, want := range map[string]bool{"x>1": false, "x<1": false, "x=1": false, "x!=1": true, "x>=NaN": false} {
		f, _ := ParseFilter(src)
		if f.Match(e) != want {
			t.Errorf("%s: want %v", src, want)
		}
	}
}

func TestAFilterErrorSpansItsTerm(t *testing.T) {
	_, err := ParseFilter("kind:host  status:bad x=1")
	var fe *FilterError
	if !errors.As(err, &fe) || fe.Col != 12 || fe.Len != len("status:bad") {
		t.Errorf("err %v; want column 12 spanning status:bad", err)
	}
}

func TestAnEmptyFilterIsZero(t *testing.T) {
	for src, want := range map[string]bool{"": true, "  ": true, "status:ok": false, "#a": false} {
		if f, _ := ParseFilter(src); f.IsZero() != want {
			t.Errorf("%q: IsZero %v", src, !want)
		}
	}
}

func TestAMetricTermMatchesWhatOffersTheMetric(t *testing.T) {
	offers := func(e *model.Entity, metric string) bool {
		return e.Kind == model.KindHost && metric == "cpu.utilisation"
	}
	host, pod := &model.Entity{Kind: model.KindHost}, &model.Entity{Kind: model.KindPod}
	cases := []struct {
		src       string
		host, pod bool
	}{
		{"metric:cpu.utilisation", true, false},
		{"metric:mem.used,cpu.utilisation", true, false}, // any of them
		{"metric:mem.used", false, false},
		{"metric:cpu.utilisation kind:pod", false, false},
	}
	for _, c := range cases {
		f, err := ParseFilter(c.src)
		if err != nil {
			t.Fatal(err)
		}
		f.Offers = offers
		if f.Match(host) != c.host || f.Match(pod) != c.pod {
			t.Errorf("%q: host %v, pod %v; want %v, %v", c.src, f.Match(host), f.Match(pod), c.host, c.pod)
		}
	}
	if f, _ := ParseFilter("metric:cpu.utilisation"); f.Match(host) {
		t.Error("with nothing to say what offers a metric, a metric term should match nothing")
	}
}

func TestASameAsTermMatchesTheRefAndWhatIsLinkedToIt(t *testing.T) {
	linked := func(e *model.Entity, ref model.EntityRef) bool {
		return ref == "prom/host/db-07" && e.Ref == "k8s/node/db-07"
	}
	f, err := ParseFilter("same_as:prom/host/db-07")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[model.EntityRef]bool{"prom/host/db-07": true, "k8s/node/db-07": false, "prom/host/web": false}
	for ref, want := range cases {
		if f.Match(&model.Entity{Ref: ref}) != want {
			t.Errorf("unlinked, %s: want %v", ref, want)
		}
	}
	f.Linked = linked
	cases["k8s/node/db-07"] = true
	for ref, want := range cases {
		if f.Match(&model.Entity{Ref: ref}) != want {
			t.Errorf("%s: want %v", ref, want)
		}
	}
	if f, _ := ParseFilter("same_as:prom/host/db-07 kind:host"); f.Match(&model.Entity{Ref: "prom/host/db-07", Kind: "node"}) {
		t.Error("same_as: should combine with the other terms")
	}
}

func TestSameAsInFollowsTheCurrentWorld(t *testing.T) {
	s := model.NewStore(model.StoreOptions{Identity: model.DefaultIdentityRules()})
	world := s.Current()
	linked := SameAsIn(func() *model.Snapshot { return world })
	a := model.Entity{Ref: "prom/host/db-07", Kind: model.KindHost, Name: "db-07", Source: "prom"}
	b := model.Entity{Ref: "k8s/node/db-07", Kind: model.KindNode, Name: "db-07.lan", Source: "k8s"}
	if linked(&b, a.Ref) {
		t.Error("linked in an empty world")
	}
	world, _ = s.Apply(&model.ChangeSet{Upserts: []model.Entity{a, b}})
	if !linked(&b, a.Ref) || !linked(&a, b.Ref) {
		t.Error("not linked once both are in the world")
	}
	world, _ = s.Apply(&model.ChangeSet{Removes: []model.EntityRef{a.Ref}})
	if linked(&b, a.Ref) {
		t.Error("still linked after one left")
	}
}

func TestUnitComparisonsReadTheQuantity(t *testing.T) {
	host := func(v model.Value) *model.Entity {
		return &model.Entity{Ref: "m/host/h", Kind: model.KindHost, Attrs: map[string]model.Value{"memory": v, "cpu": model.Number(0.9).In(model.UnitRatio)}}
	}
	cases := []struct {
		src  string
		v    model.Value
		want bool
	}{
		{"memory>8GiB", model.Number(16 << 30).In(model.UnitBytes), true},
		{"memory>8GiB", model.Number(4 << 30).In(model.UnitBytes), false},
		{"memory>8GiB", model.Number(16 << 30), true},
		{"memory<=16GiB", model.Number(16 << 30).In(model.UnitBytes), true},
		{"memory>8ms", model.Number(16 << 30).In(model.UnitBytes), false},
		{"memory>8", model.Number(16 << 30).In(model.UnitBytes), true},
		{"cpu>=80%", model.Number(0), true},
	}
	for _, c := range cases {
		f, err := ParseFilter(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got := f.Match(host(c.v)); got != c.want {
			t.Errorf("%q on %v = %v, want %v", c.src, c.v, got, c.want)
		}
	}
}

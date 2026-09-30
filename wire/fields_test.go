package wire

import (
	"mindseye/internal/data"
	"mindseye/pkg/sdk"
	pb "mindseye/pkg/sdk/proto/modulev1"
	"reflect"
	"strings"
	"testing"
)

// TestTheContractCarriesEverySDKField fails when an SDK type gains a field, so the contract
// and these conversions are changed with it.
func TestTheContractCarriesEverySDKField(t *testing.T) {
	carried := map[reflect.Type][]string{
		reflect.TypeFor[sdk.ChangeSet]():   {"Removes", "RemoveEdges", "Upserts", "Edges", "Events"},
		reflect.TypeFor[sdk.Entity]():      {"Ref", "Kind", "Name", "Attrs", "Status", "Tags", "Source", "Seen"},
		reflect.TypeFor[sdk.Status]():      {"Level", "Reason"},
		reflect.TypeFor[sdk.EdgeKey]():     {"From", "To", "Rel"},
		reflect.TypeFor[sdk.Edge]():        {"From", "To", "Rel", "Weight", "Attrs", "Source", "Traffic"},
		reflect.TypeFor[sdk.Traffic]():     {"Rate", "Unit"},
		reflect.TypeFor[sdk.Event]():       {"ID", "Entity", "At", "Severity", "Kind", "Message", "Fields", "Source"},
		reflect.TypeFor[sdk.Info]():        {"Kind", "Version", "Description"},
		reflect.TypeFor[sdk.Config]():      {"Name", "Line", "Options"},
		reflect.TypeFor[sdk.Health]():      {"Disconnected", "Err", "Note"},
		reflect.TypeFor[sdk.Metric]():      {"Name", "Unit", "Description", "Kinds", "Native", "Extra"},
		reflect.TypeFor[sdk.SeriesQuery](): {"Entities", "Filter", "Metrics", "Window", "Step", "Agg", "Native", "Top"},
		reflect.TypeFor[sdk.SeriesRef]():   {"Entity", "Metric"},
		reflect.TypeFor[sdk.Series]():      {"Ref", "Unit", "Points"},
		reflect.TypeFor[sdk.Point]():       {"T", "V"},
		reflect.TypeFor[sdk.TimeWindow]():  {"From", "To"},
		reflect.TypeFor[sdk.EventQuery]():  {"Entities", "Window", "MinSeverity", "Kinds", "Limit"},
		// Offers and Linked are functions the host fills in; they do not cross.
		reflect.TypeFor[data.Filter]():    {"Kinds", "Sources", "Statuses", "Metrics", "SameAs", "Tags", "Attrs", "Offers", "Linked"},
		reflect.TypeFor[data.Predicate](): {"Key", "Op", "Value"},
	}
	for typ, want := range carried {
		var got []string
		for f := range typ.Fields() {
			got = append(got, f.Name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%v has fields %v; the contract carries %v", typ, got, want)
		}
	}
}

func TestTheContractsEnumsNameTheSDKsValues(t *testing.T) {
	check := func(sdkName, pbName, prefix string) {
		if want := prefix + strings.ToUpper(sdkName); pbName != want {
			t.Errorf("%s is %s on the wire", sdkName, pbName)
		}
	}
	for s := sdk.StatusUnknown; s <= sdk.StatusDown; s++ {
		check(s.String(), pb.StatusLevel(s).String(), "STATUS_LEVEL_")
	}
	for s := sdk.SevDebug; s <= sdk.SevCritical; s++ {
		check(s.String(), pb.Severity(s).String(), "SEVERITY_")
	}
	for a := sdk.AggNone; a <= sdk.AggP95; a++ {
		check(a.String(), pb.Aggregation(a).String(), "AGGREGATION_")
	}
}

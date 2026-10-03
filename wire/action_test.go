package wire

import (
	"reflect"
	"testing"
	"time"

	"wayseer.dev/sdk"
	pb "wayseer.dev/sdk/proto/modulev1"
)

func TestActionsRoundTrip(t *testing.T) {
	acts := []sdk.Action{
		{ID: "restart", Title: "Restart", Changes: "Restarts it", Kinds: []sdk.Kind{sdk.KindService, sdk.KindHost}},
		{
			ID: "scale", Title: "Scale", Changes: "Sets replicas", Kinds: []sdk.Kind{sdk.KindService},
			Params: []sdk.Param{
				sdk.IntParam("replicas", "Replicas", 0, 20),
				sdk.DurationParam("for", "For", time.Minute, time.Hour).WithDefault("5m"),
				sdk.ChoiceParam("mode", "Mode", "fast", "safe").WithDefault("safe"),
			},
		},
	}
	if got := DecodeActions(EncodeActions(acts)); !reflect.DeepEqual(got, acts) {
		t.Errorf("got %+v\nwant %+v", got, acts)
	}
	if got := DecodeActions(EncodeActions(nil)); got != nil {
		t.Errorf("no actions decoded as %+v", got)
	}
}

func TestActionRequestRoundTrip(t *testing.T) {
	ref, _ := sdk.NewEntityRef("inv", sdk.KindHost, "web-01")
	for _, req := range []sdk.ActionRequest{
		{Instance: "inv", Action: "scale", Entity: ref, Params: map[string]string{"replicas": "3", "for": "5m"}},
		{Instance: "inv", Action: "restart", Entity: ref},
	} {
		if got := DecodeActionRequest(EncodeActionRequest(req)); !reflect.DeepEqual(got, req) {
			t.Errorf("got %+v, want %+v", got, req)
		}
	}
}

// A parameter type this build does not know decodes as none, which ValidateActions refuses.
func TestAnUnknownParamTypeIsRefused(t *testing.T) {
	r := &pb.ActionsResponse{Actions: []*pb.Action{{
		Id: "x", Title: "X", Changes: "Changes", Kinds: []string{"host"},
		Params: []*pb.Param{{Name: "p", Title: "P", Type: pb.ParamType(9)}},
	}}}
	if err := sdk.ValidateActions(DecodeActions(r)); err == nil {
		t.Error("an unknown parameter type was accepted")
	}
}

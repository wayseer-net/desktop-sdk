package serve

import (
	"context"
	"errors"
	"fmt"
	"mindseye/pkg/sdk"
	pb "mindseye/pkg/sdk/proto/modulev1"
	"mindseye/pkg/sdk/wire"
	"reflect"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// bare implements only the required methods, and panics when configured.
type bare struct{}

func (bare) Info() sdk.Info                              { return sdk.Info{Kind: "bare", Version: "1"} }
func (bare) Configure(context.Context, sdk.Config) error { panic("boom") }
func (bare) Run(ctx context.Context, _ sdk.Sink) error   { <-ctx.Done(); return nil }
func (bare) Health() sdk.Health                          { return sdk.Health{} }

func TestAModuleWithoutOptionalMethodsSaysSo(t *testing.T) {
	s := NewServer(bare{})
	info, _ := s.Info(context.Background(), &pb.InfoRequest{})
	if len(info.Capabilities) != 0 || info.Module.Major != wire.Protocol.Major {
		t.Errorf("info %+v", info)
	}
	_, err := s.Discover(context.Background(), &pb.DiscoverRequest{})
	if status.Code(err) != codes.Unimplemented || status.Convert(err).Message() != "the module does not offer discovery" {
		t.Errorf("discover: %v", err)
	}
}

func TestAPanicInConfigureIsAnError(t *testing.T) {
	_, err := NewServer(bare{}).Configure(context.Background(), &pb.ConfigureRequest{Name: "b"})
	if status.Code(err) != codes.InvalidArgument || status.Convert(err).Message() != "panic: boom" {
		t.Errorf("configure: %v", err)
	}
}

func TestALargeSnapshotIsSentInPartsThatJoin(t *testing.T) {
	cs := &sdk.ChangeSet{}
	for i := range 20_000 {
		ref, _ := sdk.NewEntityRef("m", sdk.KindHost, fmt.Sprint("h", i))
		cs.Upserts = append(cs.Upserts, sdk.Entity{Ref: ref, Kind: sdk.KindHost, Name: strings.Repeat("n", 60), Source: "m"})
	}
	var sent []*pb.RunResponse
	k := &streamSink{send: func(r *pb.RunResponse) error { sent = append(sent, r); return nil }}
	if err := k.Snapshot(context.Background(), cs); err != nil {
		t.Fatal(err)
	}
	if len(sent) < 2 {
		t.Fatalf("%d parts", len(sent))
	}
	var j wire.Joiner
	for _, r := range sent {
		if snapshot, got, done := j.Add(r); done && (!snapshot || !reflect.DeepEqual(wire.DecodeChangeSet(got), cs)) {
			t.Error("the parts did not join to the snapshot")
		}
	}
}

// actor offers one action, whose Do reports what it was asked or fails with fail.
type actor struct {
	bare
	fail error
}

func (actor) Actions() []sdk.Action {
	return []sdk.Action{{ID: "touch", Title: "Touch", Changes: "Marks it touched", Kinds: []sdk.Kind{sdk.KindHost}}}
}

func (a actor) Do(_ context.Context, req sdk.ActionRequest) (sdk.ActionResult, error) {
	if a.fail != nil {
		return sdk.ActionResult{}, a.fail
	}
	return sdk.ActionResult{Message: req.Action + " on " + string(req.Entity)}, nil
}

func TestAModuleWithoutActionsIsNeverAskedToAct(t *testing.T) {
	s := NewServer(bare{})
	if _, err := s.Actions(context.Background(), &pb.ActionsRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("actions: %v", err)
	}
	if _, err := s.Do(context.Background(), &pb.DoRequest{Action: "touch"}); status.Code(err) != codes.Unimplemented {
		t.Errorf("do: %v", err)
	}
}

func TestAnActorListsAndRunsItsActions(t *testing.T) {
	s := NewServer(actor{})
	info, _ := s.Info(context.Background(), &pb.InfoRequest{})
	if !slices.Contains(info.Capabilities, "actions") {
		t.Errorf("capabilities %v", info.Capabilities)
	}
	acts, err := s.Actions(context.Background(), &pb.ActionsRequest{})
	if err != nil || len(acts.Actions) != 1 || acts.Actions[0].Id != "touch" {
		t.Fatalf("actions %+v, %v", acts, err)
	}
	r, err := s.Do(context.Background(), &pb.DoRequest{Instance: "b", Action: "touch", Entity: "b/host/a"})
	if err != nil || r.Message != "touch on b/host/a" {
		t.Errorf("do %+v, %v", r, err)
	}
	_, err = NewServer(actor{fail: errors.New("refused")}).Do(context.Background(), &pb.DoRequest{Action: "touch"})
	if status.Convert(err).Message() != "refused" {
		t.Errorf("a failed action: %v", err)
	}
}

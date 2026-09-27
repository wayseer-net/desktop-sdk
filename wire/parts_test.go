package wire

import (
	"fmt"
	"math/rand"
	"mindseye/pkg/sdk"
	"reflect"
	"testing"
	"testing/quick"

	"google.golang.org/protobuf/proto"
)

func TestAVeryLargeChangeSetTravelsInPartsUnderTheLimit(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	cs := &sdk.ChangeSet{}
	for i := range 100_000 {
		e := randomEntity(r)
		e.Ref, _ = sdk.NewEntityRef("m", sdk.KindHost, fmt.Sprint("h", i))
		cs.Upserts = append(cs.Upserts, e)
	}
	cs.Events = []sdk.Event{randomEvent(r)}

	parts := Parts(true, EncodeChangeSet(cs), MaxPart)
	if len(parts) < 2 {
		t.Fatalf("%d parts", len(parts))
	}
	var j Joiner
	for i, p := range parts {
		if n := proto.Size(p); n > MaxPart {
			t.Errorf("part %d is %d bytes", i, n)
		}
		snapshot, got, done := j.Add(p)
		if done != (i == len(parts)-1) {
			t.Fatalf("part %d of %d: done %v", i, len(parts), done)
		}
		if done && (!snapshot || !reflect.DeepEqual(DecodeChangeSet(got), cs)) {
			t.Error("the joined parts differ from the change set")
		}
	}
}

func TestPartsJoinToTheSameChangeSet(t *testing.T) {
	f := func(c changeSet, limit uint16) bool {
		var j Joiner
		for _, p := range Parts(false, EncodeChangeSet(c.ChangeSet), 64+int(limit)%4096) {
			if snapshot, got, done := j.Add(p); done {
				return !snapshot && reflect.DeepEqual(DecodeChangeSet(got), c.ChangeSet)
			}
		}
		return false
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestAnEmptyChangeSetIsOnePart(t *testing.T) {
	parts := Parts(false, EncodeChangeSet(&sdk.ChangeSet{}), MaxPart)
	if len(parts) != 1 || parts[0].Continued {
		t.Fatalf("%d parts", len(parts))
	}
	var j Joiner
	if _, got, done := j.Add(parts[0]); !done || !reflect.DeepEqual(DecodeChangeSet(got), &sdk.ChangeSet{}) {
		t.Error("an empty delta did not arrive")
	}
}

func TestAnItemLargerThanTheLimitTravelsAlone(t *testing.T) {
	big := sdk.Entity{Ref: "m/host/a", Kind: sdk.KindHost, Name: string(make([]byte, 500)), Source: "m"}
	cs := &sdk.ChangeSet{Removes: []sdk.EntityRef{"m/host/b"}, Upserts: []sdk.Entity{big}}
	if parts := Parts(false, EncodeChangeSet(cs), 100); len(parts) != 2 {
		t.Errorf("%d parts, want the removal and the entity apart", len(parts))
	}
}

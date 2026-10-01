package sdktest

import (
	"context"
	"testing"
	"wayseer/pkg/sdk"
)

func TestFramesCommitOncePerFlush(t *testing.T) {
	f := NewFrames("m")
	r, _ := sdk.NewEntityRef("m", sdk.KindHost, "a")
	ctx := context.Background()
	for _, name := range []string{"first", "second", "third"} {
		e := sdk.Entity{Ref: r, Kind: sdk.KindHost, Name: name, Source: "m"}
		if err := f.Delta(ctx, &sdk.ChangeSet{Upserts: []sdk.Entity{e}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := f.Entity(r); ok {
		t.Error("an entity before any flush")
	}
	if err := f.Flush(); err != nil {
		t.Fatal(err)
	}
	if e, ok := f.Entity(r); !ok || e.Name != "third" {
		t.Errorf("entity %+v, %v; want the last upsert", e, ok)
	}
	if queued, commits := f.Counts(); queued != 3 || commits != 1 {
		t.Errorf("%d queued, %d commits; want 3 and 1", queued, commits)
	}
}

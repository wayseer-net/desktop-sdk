package sdktest

import (
	"os"
	"path/filepath"
	"testing"
	"wayseer/pkg/sdk"
)

func TestARecordedWorldLoadsBack(t *testing.T) {
	host, _ := sdk.NewEntityRef("lab", sdk.KindHost, "a")
	svc, _ := sdk.NewEntityRef("lab", sdk.KindService, "web")
	cs := &sdk.ChangeSet{
		Upserts: []sdk.Entity{
			{
				Ref: svc, Kind: sdk.KindService, Name: "web", Source: "lab", Status: sdk.Status{Level: sdk.StatusWarn, Reason: "slow"},
				Attrs: map[string]sdk.Value{"team": sdk.String("shop"), "replicas": sdk.Number(3)},
			},
			{Ref: host, Kind: sdk.KindHost, Name: "a", Source: "lab", Attrs: map[string]sdk.Value{}},
		},
		Edges: []sdk.Edge{{From: svc, To: host, Rel: sdk.RelRunsOn, Weight: 1, Source: "lab"}},
	}
	rec := RecordWorld(cs)
	path := filepath.Join(t.TempDir(), "w.jsonl")
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadWorld(t, path)
	if len(got.Upserts) != 2 || len(got.Edges) != 1 {
		t.Fatalf("loaded %d entities and %d edges; want 2 and 1", len(got.Upserts), len(got.Edges))
	}
	w := got.Upserts[1]
	if w.Ref != svc || w.Kind != sdk.KindService || w.Status.Level != sdk.StatusWarn || w.Attrs["team"].Str() != "shop" || w.Source != "lab" {
		t.Errorf("web loaded as %+v", w)
	}
	if _, ok := w.Attrs["replicas"]; ok {
		t.Error("a number attribute was recorded; only strings are")
	}
	if again := RecordWorld(&got); again != rec {
		t.Errorf("recording the loaded world differs:\n%s\nwant\n%s", again, rec)
	}
}

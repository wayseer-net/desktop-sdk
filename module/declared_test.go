package module

import (
	"strings"
	"testing"

	"wayseer.dev/sdk/manifest"
	"wayseer.dev/sdk/model"
)

func TestKeepDeclaredSaysWhatItLeavesOut(t *testing.T) {
	cat := []Action{scale, restart, {ID: "drain", Kinds: []model.Kind{model.KindHost, model.KindNode}}}
	kept, left := KeepDeclared(cat, []Action{
		{ID: "scale", Kinds: []model.Kind{model.KindService}},
		{ID: "drain", Kinds: []model.Kind{model.KindNode, model.KindPod}},
	})
	var ids []string
	for _, a := range kept {
		ids = append(ids, a.ID+" on "+string(a.Kinds[0]))
	}
	if got := strings.Join(ids, ", "); got != "scale on service, drain on node" || len(kept[1].Kinds) != 1 {
		t.Errorf("kept %q", got)
	}
	if got := strings.Join(left, ", "); got != "restart, drain on host" {
		t.Errorf("left out %q", got)
	}
	if cat[2].Kinds[0] != model.KindHost {
		t.Error("KeepDeclared changed the catalogue")
	}
}

func TestDeclaredInReadsTheManifestsActions(t *testing.T) {
	got := DeclaredIn(manifest.Manifest{Actions: []manifest.Action{{ID: "drain", Title: "Drain", Changes: "Moves pods off", Kinds: []string{"node", "acme/rack"}}}})
	if len(got) != 1 || got[0].ID != "drain" || got[0].Title != "Drain" || got[0].Changes != "Moves pods off" ||
		len(got[0].Kinds) != 2 || got[0].Kinds[1] != "acme/rack" {
		t.Errorf("DeclaredIn = %+v", got)
	}
}

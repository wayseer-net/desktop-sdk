package model

import (
	"slices"
	"testing"
)

// groupedWorld is db-07 from three sources: prom's host with one edge, k8s's node with two,
// and a cloud VM with none; and web-01, alone.
func groupedWorld(t *testing.T) *Snapshot {
	t.Helper()
	a := host("prom", "a", "db-07", nil)
	a.Status = Status{Level: StatusCrit, Reason: "disk full"}
	b := host("k8s", "b", "db-07.lan", nil)
	c := host("cloud", "c", "db-07", nil)
	d := host("prom", "d", "web-01", nil)
	p, q := host("k8s", "p", "pod-1", nil), host("k8s", "q", "pod-2", nil)
	return mustApply(t, identityStore(), &ChangeSet{
		Upserts: []Entity{a, b, c, d, p, q},
		Edges: []Edge{
			{From: a.Ref, To: d.Ref, Rel: RelTalksTo},
			{From: p.Ref, To: b.Ref, Rel: RelRunsOn},
			{From: q.Ref, To: b.Ref, Rel: RelRunsOn},
		},
	})
}

func TestGroupsFaceIsTheBestConnectedMember(t *testing.T) {
	g := GroupsOf(groupedWorld(t))
	face := mustRef("k8s", "host", "b")
	want := []EntityRef{face, mustRef("cloud", "host", "c"), mustRef("prom", "host", "a")}
	for _, r := range want {
		if g.Face(r) != face || !slices.Equal(g.Members(r), want) {
			t.Errorf("%s: face %s, members %v; want %s, %v", r, g.Face(r), g.Members(r), face, want)
		}
		if g.Hidden(r) != (r != face) {
			t.Errorf("%s hidden %v", r, g.Hidden(r))
		}
	}
	web := mustRef("prom", "host", "d")
	if g.Face(web) != web || g.Members(web) != nil || g.Hidden(web) || g.Len() != 1 {
		t.Errorf("web-01: face %s, members %v, hidden %v; %d groups", g.Face(web), g.Members(web), g.Hidden(web), g.Len())
	}
}

func TestGroupsWorstIsTheWorstMember(t *testing.T) {
	g := GroupsOf(groupedWorld(t))
	if e := g.Worst(mustRef("k8s", "host", "b")); e == nil || e.Status.Reason != "disk full" {
		t.Errorf("worst %+v", e)
	}
	web := mustRef("prom", "host", "d")
	if e := g.Worst(web); e == nil || e.Ref != web {
		t.Errorf("an ungrouped entity's worst is %+v", e)
	}
}

func TestNoGroupsLeaveEveryEntityItsOwn(t *testing.T) {
	var g *Groups
	if r := mustRef("prom", "host", "a"); g.Face(r) != r || g.Members(r) != nil || g.Hidden(r) || g.Len() != 0 {
		t.Error("nil groups changed an entity")
	}
}

func TestAProcessGivesTheFaceToWhatItRuns(t *testing.T) {
	pid := map[string]Value{"local.pid": Number(4242)}
	app := mustRef("wayseer", KindProcess, "wayseer")
	module := Entity{Ref: mustRef("wayseer", "wayseer/module", "sky-provo"), Kind: "wayseer/module", Name: "sky-provo", Source: "wayseer", Attrs: pid}
	proc := Entity{Ref: mustRef("localhost", KindProcess, "4242"), Kind: KindProcess, Name: "module", Source: "localhost", Attrs: pid}
	machine := host("localhost", "box", "box", nil)
	g := GroupsOf(mustApply(t, identityStore(), &ChangeSet{
		Upserts: []Entity{{Ref: app, Kind: KindProcess, Name: "Wayseer", Source: "wayseer"}, module, proc, machine},
		Edges: []Edge{
			{From: module.Ref, To: app, Rel: RelRunsOn},
			{From: proc.Ref, To: machine.Ref, Rel: RelRunsOn},
			{From: app, To: proc.Ref, Rel: RelParentOf},
		},
	}))
	if face := g.Face(proc.Ref); face != module.Ref {
		t.Errorf("face %s; want the module the process runs, though the process has more edges", face)
	}
}

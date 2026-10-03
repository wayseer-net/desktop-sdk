package model

import (
	"strings"
	"testing"
)

func TestRefRoundTrip(t *testing.T) {
	cases := []struct{ inst, kind, native string }{
		{"prom-main", "host", "db-07"},
		{"k8s", "k8s/ingress", "default/web"},
		{"files", "table", "100%/done"},
		{"lh", "process", "systemd --user"},
	}
	for _, c := range cases {
		ref, err := NewEntityRef(c.inst, Kind(c.kind), c.native)
		if err != nil {
			t.Fatalf("NewEntityRef(%q, %q, %q): %v", c.inst, c.kind, c.native, err)
		}
		if strings.Count(string(ref), "/") != 2 {
			t.Errorf("ref %q should have exactly two separators", ref)
		}
		inst, kind, native, err := ParseEntityRef(string(ref))
		if err != nil {
			t.Fatalf("ParseEntityRef(%q): %v", ref, err)
		}
		if inst != c.inst || string(kind) != c.kind || native != c.native {
			t.Errorf("round trip %q = %q %q %q", ref, inst, kind, native)
		}
		if ref.Instance() != c.inst || ref.Module() != ModuleID(c.inst) || string(ref.Kind()) != c.kind || ref.Native() != c.native {
			t.Errorf("accessors on %q disagree with parse", ref)
		}
	}
}

func TestRefExample(t *testing.T) {
	ref, _ := NewEntityRef("prom-main", KindHost, "db-07")
	if ref != "prom-main/host/db-07" {
		t.Errorf("ref = %q, want the PLAN §5.1 form", ref)
	}
}

func TestRefRejects(t *testing.T) {
	for _, s := range []string{
		"", "a/b", "a/b/c/d", "/host/x", "a//x", "a/host/",
		"a/host/%2", "a/host/%zz", "a/host/%41", // bad or non-canonical escapes
		"A B/host/x", "a/Host/x", // instance and kind vocabularies
	} {
		if _, _, _, err := ParseEntityRef(s); err == nil {
			t.Errorf("ParseEntityRef(%q) accepted", s)
		}
	}
	if _, err := NewEntityRef("a", KindHost, ""); err == nil {
		t.Error("empty native id accepted")
	}
}

func FuzzParseEntityRef(f *testing.F) {
	for _, s := range []string{"prom-main/host/db-07", "k8s/k8s%2Fingress/a%2Fb", "a/b/%25", "x/y/%zz"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		inst, kind, native, err := ParseEntityRef(s)
		if err != nil {
			return
		}
		ref, err := NewEntityRef(inst, kind, native)
		if err != nil {
			t.Fatalf("parsed %q but cannot rebuild: %v", s, err)
		}
		if string(ref) != s {
			t.Fatalf("parse of %q is not canonical: rebuilt %q", s, ref)
		}
	})
}

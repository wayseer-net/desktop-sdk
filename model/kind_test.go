package model

import (
	"slices"
	"testing"
	"wayseer/pkg/sdk/manifest"
)

func TestKindValidity(t *testing.T) {
	for _, k := range []Kind{KindHost, "k8s/ingress", "gpu_die", "x-1"} {
		if err := k.Validate(); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
	for _, k := range []Kind{"", "Host", "a/b/c", "/x", "x/", "a b"} {
		if k.Validate() == nil {
			t.Errorf("%q accepted", k)
		}
	}
}

func TestCoreVocabulary(t *testing.T) {
	if !KindHost.IsCore() || Kind("k8s/ingress").IsCore() {
		t.Error("core kinds misclassified")
	}
	if !RelRunsOn.IsCore() || !RelSameAs.IsCore() || Relation("k8s/selects").IsCore() {
		t.Error("core relations misclassified")
	}
	if Kind("k8s/ingress").Namespace() != "k8s" || KindHost.Namespace() != "" {
		t.Error("namespace")
	}
}

func TestManifestCoreKindsMatch(t *testing.T) {
	if got, want := manifest.CoreKinds(), CoreKinds(); !slices.Equal(got, want) {
		t.Errorf("the manifest's core kinds %v; want the model's %v", got, want)
	}
}

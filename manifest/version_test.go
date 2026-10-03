package manifest

import (
	"slices"
	"testing"
)

func TestCompareVersionsFollowsSemverPrecedence(t *testing.T) {
	// semver.org §11's own example, in order, then numbers that sort wrongly as text.
	ordered := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2",
		"1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.2.0", "1.10.0", "2.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			want := cmpInt(i, j)
			if got := CompareVersions(ordered[i], ordered[j]); got != want {
				t.Errorf("CompareVersions(%s, %s) = %d; want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if CompareVersions("1.0.0+a", "1.0.0+b") != 0 {
		t.Error("build metadata changed precedence")
	}
	shuffled := []string{"1.10.0", "1.0.0-rc.1", "2.0.0", "1.0.0-alpha", "1.2.0"}
	slices.SortFunc(shuffled, CompareVersions)
	if want := []string{"1.0.0-alpha", "1.0.0-rc.1", "1.2.0", "1.10.0", "2.0.0"}; !slices.Equal(shuffled, want) {
		t.Errorf("sorted %q; want %q", shuffled, want)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestValidIDAndVersionAreTheManifestsRules(t *testing.T) {
	for id, want := range map[string]bool{
		"acme/widget": true, "acme": false, "acme/": false, "../x": false, "acme/../x": false,
		"Acme/widget": false, "acme/widget/x": false, `acme\widget`: false,
	} {
		if ValidID(id) != want {
			t.Errorf("ValidID(%q) = %v; want %v", id, !want, want)
		}
	}
	for v, want := range map[string]bool{"1.4.0": true, "1.4.0-rc.1+b5": true, "v1.4.0": false, "1.4": false, "..": false} {
		if ValidVersion(v) != want {
			t.Errorf("ValidVersion(%q) = %v; want %v", v, !want, want)
		}
	}
}

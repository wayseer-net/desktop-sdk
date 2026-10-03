package manifest

import (
	"errors"
	"strings"
	"testing"
)

const unfilled = "id: acme/tool\nname: Tool\nversion: 0.1.0\ncontract: 1\nnamespace: acme\n"

var platformFields = Platform{OS: "linux", Arch: "arm64", SHA256: strings.Repeat("ab", 32)}

func TestFillAddsThePlatformFields(t *testing.T) {
	out, m, err := Fill([]byte(unfilled), platformFields)
	if err != nil {
		t.Fatal(err)
	}
	if m.OS != "linux" || m.Arch != "arm64" || m.SHA256 != platformFields.SHA256 {
		t.Errorf("filled %+v; want the platform's fields", m)
	}
	again, err := Parse(out)
	if err != nil || again.SHA256 != m.SHA256 || again.ID != "acme/tool" {
		t.Errorf("Fill wrote\n%s\nwhich parses as %+v, %v", out, again, err)
	}
}

func TestFillReplacesStaleValues(t *testing.T) {
	stale := unfilled + "os: darwin\narch: amd64\nsha256: " + strings.Repeat("0", 64) + "\n"
	_, m, err := Fill([]byte(stale), platformFields)
	if err != nil || m.OS != "linux" || m.Arch != "arm64" || m.SHA256 != platformFields.SHA256 {
		t.Errorf("Fill over stale values: %+v, %v; want the new platform", m, err)
	}
}

func TestFillChecksTheRestStrictly(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml, field string
		want        error
	}{
		"bad version":   {strings.Replace(unfilled, "0.1.0", "0.1", 1), "line 3: version", ErrBounds},
		"unknown key":   {unfilled + "colour: red\n", "colour", ErrUnknownKey},
		"missing name":  {strings.Replace(unfilled, "name: Tool\n", "", 1), "name", ErrMissing},
		"anchor":        {unfilled + "description: &a x\n", "line 6", ErrAnchor},
		"wrong os type": {unfilled + "os: [linux]\n", "os", ErrType},
	} {
		_, _, err := Fill([]byte(tc.yaml), platformFields)
		if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: %v; want %v naming %q", name, err, tc.want, tc.field)
		}
	}
}

func TestFillRefusesABadPlatform(t *testing.T) {
	for _, p := range []Platform{
		{OS: "Linux", Arch: "amd64", SHA256: platformFields.SHA256},
		{OS: "linux", Arch: "", SHA256: platformFields.SHA256},
		{OS: "linux", Arch: "amd64", SHA256: "abc"},
	} {
		if _, _, err := Fill([]byte(unfilled), p); !errors.Is(err, ErrBounds) {
			t.Errorf("Fill with %+v: %v; want %v", p, err, ErrBounds)
		}
	}
}

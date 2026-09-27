package wire

import "testing"

func TestAModuleOfAnotherMajorVersionIsRefused(t *testing.T) {
	cases := []struct {
		module Version
		want   string
	}{
		{Version{1, 0}, ""},
		{Version{1, 7}, ""},
		{Version{2, 0}, "the module speaks contract 2.0, and this app speaks 1.0"},
		{Version{0, 9}, "the module speaks contract 0.9, and this app speaks 1.0"},
	}
	for _, c := range cases {
		err := CheckVersion(Version{1, 0}, c.module)
		if got := errText(err); got != c.want {
			t.Errorf("module %v: %q, want %q", c.module, got, c.want)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

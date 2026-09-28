package sdktest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failures records a test's failures instead of failing it.
type failures struct {
	testing.TB
	msgs []string
}

func (f *failures) Helper()                   {}
func (f *failures) Errorf(s string, a ...any) { f.msgs = append(f.msgs, fmt.Sprintf(s, a...)) }
func (f *failures) Fatalf(s string, a ...any) { f.Errorf(s, a...) }

func TestGoldenPassesWhenTheFileMatches(t *testing.T) {
	f := &failures{TB: t}
	Golden(f, "testdata/golden.txt", "hosts: 2\n")
	if len(f.msgs) > 0 {
		t.Error(f.msgs)
	}
}

func TestGoldenFailsWithWhereToFindWhatDiffered(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"testdata/golden.txt", "golden.txt differs"},
		{"testdata/absent.txt", "UPDATE_SNAPSHOTS=1"},
	} {
		f := &failures{TB: t}
		Golden(f, tc.path, "hosts: 3\n")
		if len(f.msgs) != 1 || !strings.Contains(f.msgs[0], tc.want) {
			t.Errorf("%s: %q; want one failure saying %q", tc.path, f.msgs, tc.want)
		}
	}
}

func TestGoldenSaysTheFirstLineThatDiffers(t *testing.T) {
	f := &failures{TB: t}
	Golden(f, "testdata/golden.txt", "hosts: 3\n")
	if len(f.msgs) != 1 || !strings.Contains(f.msgs[0], `line 1: want "hosts: 2", got "hosts: 3"`) {
		t.Errorf("%q; want it to name line 1 both ways", f.msgs)
	}
}

func TestFirstDiffFindsExtraAndMissingLines(t *testing.T) {
	for _, c := range []struct{ want, got, says string }{
		{"a\nb\n", "a\n", `line 2: want "b", got nothing`},
		{"a\n", "a\nb\n", `line 2: want nothing, got "b"`},
		{"a\nb\n", "a\nc\n", `line 2: want "b", got "c"`},
	} {
		if got := firstDiff(c.want, c.got); got != c.says {
			t.Errorf("firstDiff(%q, %q) = %q, want %q", c.want, c.got, got, c.says)
		}
	}
}

func TestGoldenUpdateWritesTheFile(t *testing.T) {
	t.Setenv("UPDATE_SNAPSHOTS", "1")
	path := filepath.Join(t.TempDir(), "sub", "new.txt")
	Golden(t, path, "hosts: 4\n")
	if b, err := os.ReadFile(path); err != nil || string(b) != "hosts: 4\n" {
		t.Errorf("wrote %q, %v", b, err)
	}
}

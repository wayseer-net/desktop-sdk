package sdktest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Golden compares got with the file at path, relative to the test's package; with
// UPDATE_SNAPSHOTS=1 it writes the file instead, for review.
func Golden(t testing.TB, path, got string) {
	t.Helper()
	if os.Getenv("UPDATE_SNAPSHOTS") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("%v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		t.Fatalf("missing snapshot %s; run with UPDATE_SNAPSHOTS=1 and review it", path)
	case err != nil:
		t.Fatalf("%v", err)
	case string(want) != got:
		out := filepath.Join(t.ArtifactDir(), filepath.Base(path))
		_ = os.WriteFile(out, []byte(got), 0o644)
		t.Errorf("snapshot %s differs at %s; got written to %s (diff it, then UPDATE_SNAPSHOTS=1 to accept)", path, firstDiff(string(want), got), out)
	}
}

// firstDiff says where want and got first differ, by line, for logs with no files to diff.
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	quoted := func(lines []string, i int) string {
		if i >= len(lines) || i == len(lines)-1 && lines[i] == "" {
			return "nothing"
		}
		return strconv.Quote(lines[i])
	}
	for i := range max(len(w), len(g)) {
		if a, b := quoted(w, i), quoted(g, i); a != b {
			return fmt.Sprintf("line %d: want %s, got %s", i+1, a, b)
		}
	}
	return "no line"
}

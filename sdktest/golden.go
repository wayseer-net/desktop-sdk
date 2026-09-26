package sdktest

import (
	"errors"
	"os"
	"path/filepath"
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
		t.Errorf("snapshot %s differs; got written to %s (diff it, then UPDATE_SNAPSHOTS=1 to accept)", path, out)
	}
}

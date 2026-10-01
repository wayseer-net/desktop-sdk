package sdktest

import (
	"os"
	"testing"
	"wayseer/internal/model/worldfile"
	"wayseer/pkg/sdk"
)

// RecordWorld writes cs's entities and edges, sorted, one JSON line each, so a module's world
// can be recorded once and laid out by tests that do not run the module.
func RecordWorld(cs *sdk.ChangeSet) string { return worldfile.Record(cs) }

// LoadWorld reads a world RecordWorld wrote.
func LoadWorld(t testing.TB, path string) sdk.ChangeSet {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := worldfile.Parse(raw)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return cs
}

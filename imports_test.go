package sdk_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSDKImportsNoInternal keeps the SDK free of the core, so it can move to its own repository.
func TestSDKImportsNoInternal(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-test", "wayseer/pkg/sdk/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "wayseer/internal/") {
			t.Errorf("the SDK imports %s", pkg)
		}
	}
}

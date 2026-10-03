package sdk_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSDKImportsNothingOfTheApp keeps the SDK free of the app and its modules: it links only
// itself, the standard library and third-party modules.
func TestSDKImportsNothingOfTheApp(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-test", "wayseer.dev/sdk/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "wayseer/") || strings.HasPrefix(pkg, "wayseer.dev/modules/") {
			t.Errorf("the SDK imports %s", pkg)
		}
	}
}

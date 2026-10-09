package initialization

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amuxos/agent-pack-contract/go/internal/version"
)

func TestGeneratedCheckLocksBinaryVersion(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "example-pack")
	if _, err := InitializeAgentPack(repoRoot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "ci", "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		`EXPECTED_VERSION="` + version.Version + `"`,
		`--expected-version "$EXPECTED_VERSION"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("generated check script missing %q:\n%s", want, script)
		}
	}
}

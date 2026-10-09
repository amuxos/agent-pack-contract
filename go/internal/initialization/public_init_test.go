package initialization

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
)

func TestNewPackAcceptsContentOnlyProfileAndPublicCI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "example-pack")
	if _, err := InitializeAgentPack(root); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(root, "profiles", "common", "reviewer.toml")
	if err := os.MkdirAll(filepath.Dir(profile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte("description = \"Review changes.\"\ninstructions = []\nskills = []\nsubagents = []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest, err := contract.BuildAgentPackManifest(root)
	if err != nil {
		t.Fatalf("new pack must accept a content-only profile: %v", err)
	}
	if schema, _ := manifest.Get("schema_version"); schema != "v2" {
		t.Fatalf("new pack schema = %v, want v2", schema)
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "check.yml")); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(root, "ci", "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "https://github.com/amuxos/agent-pack-contract-releases/releases/download/v") {
		t.Fatal("generated CI does not use a pinned public release")
	}
}

package rendering

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/testutil"
)

func TestPortableRenderRequiresExecutionAndProjectsBothEngines(t *testing.T) {
	root := testutil.FixturePath(t, "portable-pack")
	m, err := contract.BuildAgentPackManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	if err := contract.WriteJSON(manifest, m); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "workspace")
	if err := RenderLocalWorkspace(root, "careful", out, manifest, ""); err == nil || !strings.Contains(err.Error(), "--agent is required") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("invalid render mutated output")
	}
	for _, agent := range []string{"codex", "claude-code"} {
		t.Run(agent, func(t *testing.T) {
			if err := RenderLocalWorkspaceWithOptions(root, "careful", out, manifest, ExecutionOptions{Agent: agent, Model: "native-model", PermissionMode: "plan"}); err != nil {
				t.Fatal(err)
			}
			layout := LocalWorkspaceAgents[agent]
			for _, path := range []string{layout.InstructionsFile, filepath.Join(layout.SkillsDir, "review/SKILL.md"), filepath.Join(layout.SubagentsDir, "checker.md")} {
				if _, err := os.Stat(filepath.Join(out, path)); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(filepath.Join(out, layout.ModelConfigFile))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "native-model") {
				t.Fatalf("missing model: %s", data)
			}
			want := "plan"
			if agent == "codex" {
				want = "read-only"
			}
			if !strings.Contains(string(data), want) {
				t.Fatalf("missing permission: %s", data)
			}
		})
	}
	if err := RenderLocalWorkspaceWithOptions(root, "careful", out, manifest, ExecutionOptions{Agent: "pi", PermissionMode: "bypass"}); err == nil {
		t.Fatal("unsupported permission mode accepted")
	}
	for _, agent := range []string{"codex", "claude", "pi"} {
		if err := RenderLocalWorkspaceWithOptions(root, "careful", out, manifest, ExecutionOptions{Agent: agent, Model: "example/review"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, "workspace-profile.json"))
		if err != nil {
			t.Fatal(err)
		}
		var workspace map[string]any
		if err := json.Unmarshal(data, &workspace); err != nil {
			t.Fatal(err)
		}
		if workspace["model"] != "example/review" {
			t.Fatalf("lost caller model: %s", data)
		}
		for _, key := range []string{"provider", "providers"} {
			if _, present := workspace[key]; present {
				t.Fatalf("v2 render injected %s: %s", key, data)
			}
		}
		if _, err := os.Stat(filepath.Join(out, ".pi/extensions/agent-pack-provider.ts")); !os.IsNotExist(err) {
			t.Fatal("v2 emitted a provider extension")
		}
	}
	// Even an empty legacy field must fail before changing existing output.
	m.Set("providers", map[string]any{})
	if err := contract.WriteJSON(manifest, m); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(out, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RenderLocalWorkspaceWithOptions(root, "careful", out, manifest, ExecutionOptions{Agent: "codex"}); err == nil {
		t.Fatal("accepted v2 provider field")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("invalid render modified output")
	}
}

// Provider connection projection is still part of the v1.1 contract.
func TestLegacyProviderCatalogStillProjectsConnections(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(testutil.FixturePath(t, "portable-pack"))); err != nil {
		t.Fatal(err)
	}
	packFile := filepath.Join(root, "agent-pack.toml")
	pack, err := os.ReadFile(packFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packFile, []byte(strings.Replace(string(pack), `"v2"`, `"v1.1"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	profileFile := filepath.Join(root, "profiles/common/reviewer.toml")
	profile, err := os.ReadFile(profileFile)
	if err != nil {
		t.Fatal(err)
	}
	profile = append([]byte("type = \"pi\"\nmodel = \"example/review\"\nproviders = [\"example/review\"]\n"), profile...)
	if err := os.WriteFile(profileFile, profile, 0600); err != nil {
		t.Fatal(err)
	}
	providerFile := filepath.Join(root, "providers/example/review.toml")
	if err := os.MkdirAll(filepath.Dir(providerFile), 0700); err != nil {
		t.Fatal(err)
	}
	provider := `platform = "openai-compat"
api_key_env = "MODEL_TOKEN"
base_url = "https://api.example.com/v1"
upstream_model = "review-model"
`
	if err := os.WriteFile(providerFile, []byte(provider), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := contract.BuildAgentPackManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := contract.WriteJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"pi"} {
		t.Run(agent, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "workspace")
			if err := RenderLocalWorkspaceWithOptions(root, "careful", out, manifestPath, ExecutionOptions{Agent: agent}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(out, "workspace-profile.json"))
			if err != nil {
				t.Fatal(err)
			}
			var workspace map[string]any
			if err := json.Unmarshal(data, &workspace); err != nil {
				t.Fatal(err)
			}
			if workspace["provider"] == nil || len(workspace["providers"].([]any)) != 1 {
				t.Fatalf("lost legacy provider: %s", data)
			}
			configFile := filepath.Join(out, LocalWorkspaceAgents[agent].ModelConfigFile)
			data, err = os.ReadFile(configFile)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "review-model") {
				t.Fatalf("lost upstream model: %s", data)
			}
			if agent == "pi" {
				data, err = os.ReadFile(filepath.Join(out, ".pi/extensions/agent-pack-provider.ts"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if !strings.Contains(string(data), "MODEL_TOKEN") || !strings.Contains(string(data), "https://api.example.com/v1") {
				t.Fatalf("lost legacy connection config: %s", data)
			}
		})
	}
}

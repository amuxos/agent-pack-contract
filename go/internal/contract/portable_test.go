package contract_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
	"github.com/amuxos/agent-pack-contract/go/internal/schema"
	"github.com/amuxos/agent-pack-contract/go/internal/testutil"
)

func TestPortableManifestPreservesContentWithoutProviders(t *testing.T) {
	root := testutil.FixturePath(t, "portable-pack")
	m, err := contract.BuildAgentPackManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	plain := jsonx.Plain(m).(map[string]any)
	if err := schema.Validate(plain); err != nil {
		t.Fatal(err)
	}
	profiles := plain["profiles"].(map[string]any)
	base := profiles["reviewer"].(map[string]any)
	derived := profiles["careful"].(map[string]any)
	for _, key := range []string{"instructions", "skills", "subagents", "envs"} {
		if !reflect.DeepEqual(base[key], derived[key]) {
			t.Fatalf("lost inherited %s", key)
		}
	}
	if _, present := plain["providers"]; present {
		t.Fatal("v2 manifest contains provider catalog")
	}
	for _, key := range []string{"type", "model", "providers", "supported_models", "permission"} {
		for _, value := range []any{nil, "", []any{}, map[string]any{}} {
			base[key] = value
			if err := contract.ValidateAgentPackManifest(plain); err == nil {
				t.Fatalf("semantic validator accepted %s=%v", key, value)
			}
			if err := schema.Validate(plain); err == nil {
				t.Fatalf("schema accepted %s=%v", key, value)
			}
			delete(base, key)
		}
	}
	clone := t.TempDir()
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	m2, err := contract.BuildAgentPackManifest(clone)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonx.Plain(m), jsonx.Plain(m2)) {
		t.Fatal("manifest depends on checkout path")
	}
}

func TestPortableSourceRejectsExecutionFields(t *testing.T) {
	for _, field := range []string{`type = ""`, `model = ""`, `providers = []`, `supported_models = []`, `permission = {}`} {
		t.Run(field, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS(testutil.FixturePath(t, "portable-pack"))); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "profiles/common/reviewer.toml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(data, []byte(field+"\n")...), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := contract.BuildAgentPackManifest(root); err == nil || !strings.Contains(err.Error(), "not allowed in v2") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLegacyVersionWhitespaceKeepsCanonicalOutput(t *testing.T) {
	root := testutil.SamplePackRoot(t)
	baseline, err := contract.BuildAgentPackManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	clone := t.TempDir()
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(clone, "agent-pack.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), `"v1.1"`, `" v1.1 "`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := contract.BuildAgentPackManifest(clone)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonx.Plain(baseline), jsonx.Plain(got)) {
		t.Fatal("legacy canonical output changed")
	}
}

func TestManifestProviderPresenceDependsOnVersion(t *testing.T) {
	for _, version := range []string{"v1.1", "v2"} {
		t.Run(version, func(t *testing.T) {
			m := map[string]any{"schema_version": version, "pack_namespace": "test", "profiles": map[string]any{}, "namespaces": map[string]any{}}
			for _, present := range []bool{false, true} {
				for _, value := range []any{map[string]any{}, map[string]any{"example/model": map[string]any{"platform": "openai-compat", "api_key_env": "TOKEN"}}, nil, "", []any{}} {
					if present {
						m["providers"] = value
					} else {
						delete(m, "providers")
					}
					_, isMap := value.(map[string]any)
					wantValid := (version == "v2" && !present) || (version == "v1.1" && present && isMap)
					for name, validate := range map[string]func(any) error{"semantic": contract.ValidateAgentPackManifest, "schema": schema.Validate} {
						err := validate(m)
						if (err == nil) != wantValid {
							t.Fatalf("%s: version=%s present=%t value=%v error=%v", name, version, present, value, err)
						}
					}
				}
			}
		})
	}
}

func TestPortableSourceRejectsProviderFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(testutil.FixturePath(t, "portable-pack"))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "providers/example/model.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// Reject the execution configuration before trying to parse its contents.
	if err := os.WriteFile(path, []byte("not valid toml"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { _, err := contract.BuildAgentPackManifest(root); return err },
		func() error { _, err := contract.ValidateContract(root); return err },
	} {
		if err := check(); err == nil || !strings.Contains(err.Error(), "provider files are not allowed in v2") {
			t.Fatalf("error=%v", err)
		}
	}
}

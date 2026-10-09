package rendering

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
)

// WriteToml emits v as TOML via pelletier. Consumers rely on the parsed
// semantics rather than byte layout. v should be a struct (not a bare map) for
// stable output ordering.
func WriteToml(path string, v any) error {
	data, err := toml.Marshal(v)
	if err != nil {
		return errs.New("marshal TOML for %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteYaml emits v as YAML via yaml.v3. v should be a struct or map;
// use yaml.Node / struct tags for any field that require explicit quoting
// (headers.extra JSON-in-string, ${SESSION_ID}, ...).
func WriteYaml(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return errs.New("marshal YAML for %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

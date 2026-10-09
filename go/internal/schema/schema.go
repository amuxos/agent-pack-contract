// Package schema validates manifests against the packaged JSON Schema.
package schema

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
)

//go:embed agent-pack-manifest.schema.json
var schemaJSON []byte

const resourceURL = "agent-pack-manifest.schema.json"

// Validate checks a decoded manifest against the packaged JSON Schema
// (draft 2020-12).
func Validate(manifest any) error {
	var doc any
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return errs.New("invalid packaged manifest schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resourceURL, doc); err != nil {
		return errs.New("invalid packaged manifest schema: %v", err)
	}
	sch, err := c.Compile(resourceURL)
	if err != nil {
		return errs.New("invalid packaged manifest schema: %v", err)
	}
	if err := sch.Validate(manifest); err != nil {
		return errs.New("manifest does not match manifest schema: %v", err)
	}
	return nil
}

// ValidateRepositorySchema rejects a checked-in compatibility schema that
// drifted from the packaged canonical schema.
func ValidateRepositorySchema(repoRoot string, required bool) error {
	schemaPath := filepath.Join(repoRoot, "schemas", "agent-pack-manifest.schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return errs.New("missing schemas/agent-pack-manifest.schema.json")
			}
			return nil
		}
		return errs.New("%s: %v", schemaPath, err)
	}
	var repo any
	if err := json.Unmarshal(data, &repo); err != nil {
		return errs.New("%s: invalid JSON: %v", schemaPath, err)
	}
	var packaged any
	if err := json.Unmarshal(schemaJSON, &packaged); err != nil {
		return errs.New("invalid packaged manifest schema: %v", err)
	}
	if !reflect.DeepEqual(repo, packaged) {
		return errs.New("%s: repository schema differs from packaged schema", schemaPath)
	}
	return nil
}

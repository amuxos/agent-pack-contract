package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
)

// ENVNameRE matches a bare environment variable name.
var ENVNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ForbiddenProviderKeys lists inline credential fields that must never appear
// in a committed profile.
var ForbiddenProviderKeys = []string{
	"api_key", "apikey", "api-key", "secret", "secret_key", "access_key", "password", "token",
}

// PermissionModes lists the permission modes defined by the Contract.
var PermissionModes = []string{"default", "plan", "bypass"}

// getField reads key from either a map[string]any (manifest JSON / plain TOML)
// or a *jsonx.OrderedMap (ordered TOML). The bool is "key present".
func getField(obj any, key string) (any, bool) {
	switch o := obj.(type) {
	case map[string]any:
		v, ok := o[key]
		return v, ok
	case *jsonx.OrderedMap:
		return o.Get(key)
	}
	return nil, false
}

func requireString(obj any, key, context string) (string, error) {
	v, _ := getField(obj, key)
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", errs.New("%s: missing or invalid string field %q", context, key)
	}
	return strings.TrimSpace(s), nil
}

func requireStringList(obj any, key, context string) ([]string, error) {
	v, present := getField(obj, key)
	if !present {
		// A missing list field defaults to an empty list.
		return []string{}, nil
	}
	if v == nil {
		// An explicitly null field is invalid rather than equivalent to omission.
		return nil, errs.New("%s: field %q must be a list of non-empty strings", context, key)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, errs.New("%s: field %q must be a list of non-empty strings", context, key)
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, errs.New("%s: field %q must be a list of non-empty strings", context, key)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func optionalStringList(obj any, key, context string) ([]string, error) {
	if _, present := getField(obj, key); !present {
		return nil, nil
	}
	return requireStringList(obj, key, context)
}

func optionalString(obj any, key, context string, allowed ...string) (string, error) {
	v, present := getField(obj, key)
	if !present {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", errs.New("%s: field %q must be a non-empty string", context, key)
	}
	s = strings.TrimSpace(s)
	if len(allowed) > 0 {
		hit := false
		for _, a := range allowed {
			if s == a {
				hit = true
				break
			}
		}
		if !hit {
			return "", errs.New("%s: field %q must be one of %s; got %q", context, key, strings.Join(allowed, ", "), s)
		}
	}
	return s, nil
}

// WriteJSON writes v in the byte-stable Contract JSON format.
func WriteJSON(path string, v any) error {
	payload, err := jsonx.Marshal(v, "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

// ReadJSON reads and decodes a JSON object. Key order is irrelevant when
// reading (only when writing), so encoding/json is fine here.
func ReadJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New("missing %s", path)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errs.New("%s: invalid JSON: %v", path, err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		return nil, errs.New("%s: expected JSON object", path)
	}
	return m, nil
}

// Exported aliases used by the rendering package.
func LoadAssets(repoRoot string) (map[string]*Asset, error) {
	return loadAssets(repoRoot)
}

func AssetRoot(repoRoot, assetType, sourceKind string) string {
	return assetRoot(repoRoot, assetType, sourceKind)
}

func AssetContentFilename(assetType string) string {
	return assetContentFilename(assetType)
}

func LoadToml(path string) (map[string]any, error) {
	return loadToml(path)
}

func ValidateContract(repoRoot string) ([]string, error) {
	return validateContract(repoRoot)
}

func RequireString(obj any, key, context string) (string, error) {
	return requireString(obj, key, context)
}

func RequireStringList(obj any, key, context string) ([]string, error) {
	return requireStringList(obj, key, context)
}

func OptionalString(obj any, key, context string, allowed ...string) (string, error) {
	return optionalString(obj, key, context, allowed...)
}

func OptionalStringList(obj any, key, context string) ([]string, error) {
	return optionalStringList(obj, key, context)
}

func GetField(obj any, key string) (any, bool) {
	return getField(obj, key)
}

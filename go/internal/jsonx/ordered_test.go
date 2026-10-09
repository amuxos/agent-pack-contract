package jsonx

import "testing"

// golden is the byte-exact Contract JSON for the structure built in
// TestMarshalStableEncoding. It exercises insertion order (non-alphabetical),
// nested objects/arrays, empty object {}, empty array [], null, int, and string
// escaping (quotes, backslash, newline, tab, control char \u0001, non-ASCII,
// and unescaped <>&).
const golden = `{
  "schema_version": "v1.1",
  "pack_namespace": "example-org.sample",
  "s": "quote\" back\\ new\n tab\t ctrl\u0001 中文 <>&",
  "profiles": {
    "reviewer": {
      "namespace": "common",
      "type": "codex",
      "model": "gpt-5.4",
      "supported_models": [
        "gpt-5.4",
        "gpt-5.3"
      ],
      "providers": [],
      "instructions": [
        "instructions/common/reviewer/AGENTS.md"
      ],
      "skills": [],
      "subagents": [],
      "extends": null,
      "permission": {
        "permission_mode": "default",
        "allowed_tools": [],
        "ask_tools": []
      }
    }
  },
  "providers": {},
  "namespaces": {
    "common": {
      "profiles": [
        "reviewer"
      ]
    }
  }
}`

func TestMarshalStableEncoding(t *testing.T) {
	root := NewMap()
	root.Set("schema_version", "v1.1")
	root.Set("pack_namespace", "example-org.sample")
	root.Set("s", "quote\" back\\ new\n tab\t ctrl\x01 中文 <>&")

	profiles := NewMap()
	reviewer := NewMap()
	reviewer.Set("namespace", "common")
	reviewer.Set("type", "codex")
	reviewer.Set("model", "gpt-5.4")
	reviewer.Set("supported_models", []any{"gpt-5.4", "gpt-5.3"})
	reviewer.Set("providers", []any{})
	reviewer.Set("instructions", []any{"instructions/common/reviewer/AGENTS.md"})
	reviewer.Set("skills", []any{})
	reviewer.Set("subagents", []any{})
	reviewer.Set("extends", nil)
	permission := NewMap()
	permission.Set("permission_mode", "default")
	permission.Set("allowed_tools", []any{})
	permission.Set("ask_tools", []any{})
	reviewer.Set("permission", permission)
	profiles.Set("reviewer", reviewer)
	root.Set("profiles", profiles)

	root.Set("providers", NewMap()) // empty object -> {}

	namespaces := NewMap()
	common := NewMap()
	common.Set("profiles", []any{"reviewer"})
	namespaces.Set("common", common)
	root.Set("namespaces", namespaces)

	got, err := Marshal(root, "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := golden + "\n"
	if string(got) != want {
		t.Errorf("jsonx.Marshal produced unstable output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

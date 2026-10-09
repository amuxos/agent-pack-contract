// Package contract holds the Agent Pack source contract types.
package contract

// Asset is an Agent Pack skill or subagent discovered from source.
type Asset struct {
	Name        string
	AssetType   string // "skill" | "subagent"
	SourceKind  string // "upstream" | "custom"
	Root        string
	ContentFile string
	SourceToml  string
	Origin      map[string]any
}

// Permission describes the policy projected into an Agent workspace.
type Permission struct {
	Mode         string
	AllowedTools []string
	AskTools     []string
}

// Provider describes one external model provider binding.
type Provider struct {
	Platform  string
	APIKeyEnv string
	// Options holds the provider's extra keys. It is *jsonx.OrderedMap when
	// read from a provider TOML file (key order must reach the manifest JSON),
	// and map[string]any when read back from a manifest JSON (order irrelevant
	// because rendering re-derives output order).
	Options any
}

// Profile is the resolved source representation used to build and render a
// manifest. Empty string means an absent optional field; a nil slice means an
// absent optional list field.
type Profile struct {
	Name             string
	Namespace        string
	ProfileType      string
	Description      string
	Model            string
	Instructions     string
	InstructionFiles []string
	Skills           []string
	Subagents        []string
	Path             string
	SupportedModels  []string
	ProviderRefs     []string
	Envs             []string
	Permission       *Permission
	Extends          string
}

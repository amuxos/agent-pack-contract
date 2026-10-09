// Package rendering renders a validated manifest into an
// agent-specific local workspace.
package rendering

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
	"github.com/amuxos/agent-pack-contract/go/internal/provider"
)

// Layout describes one local agent's file layout.
type Layout struct {
	InstructionsFile string
	SkillsDir        string
	SubagentsDir     string
	ModelConfigFile  string
	ConfigFormat     string // toml_model | json_model | json_pi
	ExtensionsDir    string // pi only
}

var LocalWorkspaceAgents = map[string]Layout{
	"codex":       {InstructionsFile: "AGENTS.md", SkillsDir: ".agents/skills", SubagentsDir: ".agents/agents", ModelConfigFile: ".codex/config.toml", ConfigFormat: "toml_model"},
	"claude-code": {InstructionsFile: "CLAUDE.md", SkillsDir: ".claude/skills", SubagentsDir: ".claude/agents", ModelConfigFile: ".claude/settings.json", ConfigFormat: "json_model"},
	"pi":          {InstructionsFile: "AGENTS.md", SkillsDir: ".pi/skills", SubagentsDir: ".pi/agents", ModelConfigFile: ".pi/settings.json", ConfigFormat: "json_pi", ExtensionsDir: ".pi/extensions"},
}

var ProfileTypeToLocalAgent = map[string]string{
	"claude":      "claude-code",
	"claude-code": "claude-code",
	"codex":       "codex",
	"pi":          "pi",
}

var claudePermissionMode = map[string]string{"default": "default", "plan": "plan", "bypass": "bypassPermissions"}

var codexPermissionMode = map[string][2]string{
	"default": {"workspace-write", "never"},
	"plan":    {"read-only", "on-request"},
	"bypass":  {"danger-full-access", "never"},
}

// --- helpers ----------------------------------------------------------------

func hasProvider(m map[string]contract.Provider, key string) bool {
	_, ok := m[key]
	return ok
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func strList(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func piProviderID(ref string) string {
	if i := strings.Index(ref, "/"); i >= 0 {
		return ref[:i]
	}
	return ref
}

// --- manifest-entry readers -------------------------------------------------

func permissionFromManifest(raw any, context string) (*contract.Permission, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, nil
	}
	mode, err := contract.OptionalString(m, "permission_mode", context, contract.PermissionModes...)
	if err != nil {
		return nil, err
	}
	allowed, err := contract.RequireStringList(m, "allowed_tools", context)
	if err != nil {
		return nil, err
	}
	ask, err := contract.RequireStringList(m, "ask_tools", context)
	if err != nil {
		return nil, err
	}
	return &contract.Permission{Mode: mode, AllowedTools: allowed, AskTools: ask}, nil
}

func providerFromManifest(raw map[string]any, context string) (contract.Provider, error) {
	platform, err := contract.RequireString(raw, "platform", context)
	if err != nil {
		return contract.Provider{}, err
	}
	if !containsStr(provider.SupportedPlatforms, platform) {
		return contract.Provider{}, errs.New("%s: unknown platform %q; supported values: %s",
			context, platform, strings.Join(provider.SupportedPlatforms, ", "))
	}
	apiKeyEnv, err := contract.RequireString(raw, "api_key_env", context)
	if err != nil {
		return contract.Provider{}, err
	}
	if !contract.ENVNameRE.MatchString(apiKeyEnv) {
		return contract.Provider{}, errs.New(
			"%s: 'api_key_env' must be a bare environment variable name (matching %s); got %q",
			context, contract.ENVNameRE.String(), apiKeyEnv)
	}
	options := map[string]any{}
	for k, v := range raw {
		if k != "platform" && k != "api_key_env" {
			options[k] = v
		}
	}
	return contract.Provider{Platform: platform, APIKeyEnv: apiKeyEnv, Options: options}, nil
}

func profileFromManifestEntry(repoRoot, profileID string, entry map[string]any) (*contract.Profile, error) {
	instructionValues, err := contract.RequireStringList(entry, "instructions", profileID)
	if err != nil {
		return nil, err
	}
	var instructionFiles []string
	for _, item := range instructionValues {
		instructionFiles = append(instructionFiles, filepath.Join(repoRoot, filepath.FromSlash(item)))
	}
	instructions := ""
	if len(instructionFiles) > 0 {
		instructions = instructionFiles[0]
	}
	namespace, err := contract.RequireString(entry, "namespace", profileID)
	if err != nil {
		return nil, err
	}
	profileType, err := contract.OptionalString(entry, "type", profileID)
	if err != nil {
		return nil, err
	}
	model, err := contract.OptionalString(entry, "model", profileID)
	if err != nil {
		return nil, err
	}
	skills, err := contract.RequireStringList(entry, "skills", profileID)
	if err != nil {
		return nil, err
	}
	subagents, err := contract.RequireStringList(entry, "subagents", profileID)
	if err != nil {
		return nil, err
	}
	providerRefs, err := contract.RequireStringList(entry, "providers", profileID)
	if err != nil {
		return nil, err
	}
	envs, err := contract.RequireStringList(entry, "envs", profileID)
	if err != nil {
		return nil, err
	}
	supported, err := contract.OptionalStringList(entry, "supported_models", profileID)
	if err != nil {
		return nil, err
	}
	var permissionRaw any
	if v, ok := entry["permission"]; ok {
		permissionRaw = v
	}
	permission, err := permissionFromManifest(permissionRaw, profileID+".permission")
	if err != nil {
		return nil, err
	}
	extends := ""
	if v, ok := entry["extends"].(string); ok {
		extends = v
	}
	return &contract.Profile{
		Name:             profileID,
		Namespace:        namespace,
		ProfileType:      profileType,
		Model:            model,
		Instructions:     instructions,
		InstructionFiles: instructionFiles,
		Skills:           skills,
		Subagents:        subagents,
		Path:             profileID,
		SupportedModels:  supported,
		ProviderRefs:     providerRefs,
		Envs:             envs,
		Permission:       permission,
		Extends:          extends,
	}, nil
}

func resolveLocalAgent(profileType string) (string, error) {
	candidate := strings.TrimSpace(profileType)
	if candidate == "" {
		return "", errs.New("profile type is required to resolve local workspace layout")
	}
	agent, ok := ProfileTypeToLocalAgent[candidate]
	if !ok {
		keys := make([]string, 0, len(ProfileTypeToLocalAgent))
		for k := range ProfileTypeToLocalAgent {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", errs.New("unsupported profile type or local agent %q; supported values: %s",
			candidate, strings.Join(keys, ", "))
	}
	return agent, nil
}

// --- instructions / assets --------------------------------------------------

func renderProfileInstructions(profile *contract.Profile) (string, error) {
	var rendered []string
	if len(profile.InstructionFiles) > 0 {
		var contents []string
		for _, f := range profile.InstructionFiles {
			data, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			contents = append(contents, strings.TrimRight(string(data), " \t\r\n"))
		}
		rendered = append(rendered, strings.Join(contents, "\n\n"))
		rendered = append(rendered, "")
	}
	skills := "(none)"
	if len(profile.Skills) > 0 {
		skills = strings.Join(profile.Skills, ", ")
	}
	subagents := "(none)"
	if len(profile.Subagents) > 0 {
		subagents = strings.Join(profile.Subagents, ", ")
	}
	rendered = append(rendered, "## Bound Assets", "", "- skills: "+skills, "- subagents: "+subagents)
	return strings.Join(rendered, "\n") + "\n", nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func renderLocalSubagent(asset *contract.Asset, targetFile string) error {
	data, err := os.ReadFile(asset.ContentFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(targetFile, append([]byte(strings.TrimRight(string(data), " \t\r\n")), '\n'), 0o644); err != nil {
		return err
	}
	bundleDir := strings.TrimSuffix(targetFile, filepath.Ext(targetFile))
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(asset.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(asset.Root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "AGENTS.md" || relSlash == "source.toml" {
			return nil
		}
		return copyFile(path, filepath.Join(bundleDir, rel))
	})
}

// --- config serializers -----------------------------------------------------

type approvalSandboxConfig struct {
	Model          string `toml:"model,omitempty"`
	ApprovalPolicy string `toml:"approval_policy,omitempty"`
	SandboxMode    string `toml:"sandbox_mode,omitempty"`
}

func approvalSandboxConfigFor(model string, permission *contract.Permission) approvalSandboxConfig {
	cfg := approvalSandboxConfig{Model: model}
	if permission != nil && permission.Mode != "" {
		m := codexPermissionMode[permission.Mode]
		cfg.ApprovalPolicy = m[1]
		cfg.SandboxMode = m[0]
	}
	return cfg
}

func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// --- JSON configs (byte-critical via jsonx) ---------------------------------

func buildClaudeConfig(model string, permission *contract.Permission) *jsonx.OrderedMap {
	cfg := jsonx.NewMap()
	if model != "" {
		cfg.Set("model", model)
	}
	if permission != nil {
		perms := jsonx.NewMap()
		if permission.Mode != "" {
			perms.Set("defaultMode", claudePermissionMode[permission.Mode])
		}
		if len(permission.AllowedTools) > 0 {
			perms.Set("allow", strList(permission.AllowedTools))
		}
		if len(permission.AskTools) > 0 {
			perms.Set("ask", strList(permission.AskTools))
		}
		if perms.Len() > 0 {
			cfg.Set("permissions", perms)
		}
	}
	return cfg
}

func buildPiConfig(model string, providerTuple []string) *jsonx.OrderedMap {
	cfg := jsonx.NewMap()
	if len(providerTuple) == 2 {
		cfg.Set("defaultProvider", providerTuple[0])
		cfg.Set("defaultModel", providerTuple[1])
	} else if model != "" {
		cfg.Set("defaultModel", model)
	}
	return cfg
}

// --- pi provider extension (byte-critical TS) ------------------------------

type piRegistration struct {
	ID      string
	Name    string
	APIKey  string
	BaseURL string
	API     string
	Models  []*jsonx.OrderedMap
}

func buildPiProviderRegistrations(profile *contract.Profile, bindings map[string]contract.Provider) ([]piRegistration, error) {
	refs := append([]string{}, profile.ProviderRefs...)
	if profile.Model != "" && !containsStr(profile.ProviderRefs, profile.Model) {
		if _, ok := bindings[profile.Model]; ok {
			refs = append(refs, profile.Model)
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	groups := map[string][]struct {
		ref  string
		prov contract.Provider
	}{}
	for _, ref := range refs {
		prov, ok := bindings[ref]
		if !ok {
			return nil, errs.New("%s: provider-backed model %q has no provider config", profile.Path, ref)
		}
		pid := piProviderID(ref)
		groups[pid] = append(groups[pid], struct {
			ref  string
			prov contract.Provider
		}{ref, prov})
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var regs []piRegistration
	for _, pid := range ids {
		group := groups[pid]
		var modelEntries []*jsonx.OrderedMap
		var connMap map[string]any
		apiKeyEnv := ""
		for _, item := range group {
			entry, conn, err := provider.BuildPiProviderEntry(item.prov.Platform, item.prov.Options, item.prov.APIKeyEnv, item.ref)
			if err != nil {
				return nil, err
			}
			plain := jsonx.Plain(conn).(map[string]any)
			if connMap != nil && !reflect.DeepEqual(connMap, plain) {
				return nil, errs.New("%s: provider %q has conflicting connection config across refs", profile.Path, pid)
			}
			connMap = plain
			apiKeyEnv = item.prov.APIKeyEnv
			modelEntries = append(modelEntries, entry)
		}
		if connMap == nil {
			continue
		}
		regs = append(regs, piRegistration{
			ID:      pid,
			Name:    pid,
			APIKey:  "$" + apiKeyEnv,
			BaseURL: getStr(connMap, "baseUrl"),
			API:     getStr(connMap, "api"),
			Models:  modelEntries,
		})
	}
	return regs, nil
}

func serializePiProviderExtension(regs []piRegistration) string {
	var lines []string
	lines = append(lines,
		"// Generated by agent-pack-contract. Registers agent-pack providers with pi.",
		"// Do not edit manually.")
	lines = append(lines, `import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";`)
	lines = append(lines, "")
	lines = append(lines, "export default function (pi: ExtensionAPI): void {")
	for _, r := range regs {
		body := jsonx.NewMap()
		body.Set("name", r.Name)
		body.Set("apiKey", r.APIKey)
		body.Set("baseUrl", r.BaseURL)
		body.Set("api", r.API)
		models := make([]any, len(r.Models))
		for i, m := range r.Models {
			models[i] = m
		}
		body.Set("models", models)
		lines = append(lines, "  pi.registerProvider("+jsonx.Quote(r.ID)+", "+string(jsonx.Encode(body, "    "))+");")
		lines = append(lines, "")
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n"
}

// --- model selection / agent config ----------------------------------------

func profileSelectedProvider(profile *contract.Profile, bindings map[string]contract.Provider) (*contract.Provider, error) {
	model := profile.Model
	if model == "" {
		return nil, nil
	}
	qualifiedBinding := strings.Contains(model, "/") && hasProvider(bindings, model)
	if !containsStr(profile.ProviderRefs, model) && !qualifiedBinding {
		return nil, nil
	}
	prov, ok := bindings[model]
	if !ok {
		return nil, errs.New("%s: provider-backed model %q has no provider config", profile.Path, model)
	}
	return &prov, nil
}

func applyModelOverride(profile *contract.Profile, modelOverride string) (*contract.Profile, error) {
	if modelOverride == "" {
		return profile, nil
	}
	model := strings.TrimSpace(modelOverride)
	if model == "" {
		return nil, errs.New("render model override must be a non-empty string")
	}
	if profile.SupportedModels != nil && !containsStr(profile.SupportedModels, model) {
		return nil, errs.New("model %q is not supported by profile %s", model, profile.Name)
	}
	copy := *profile
	copy.Model = model
	return &copy, nil
}

// renderLocalAgentConfig writes the model config file and returns its relative path.
func renderLocalAgentConfig(layout Layout, profile *contract.Profile, prov *contract.Provider, outDir string) (string, error) {
	relPath := layout.ModelConfigFile
	target := filepath.Join(outDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	permission := profile.Permission
	model := profile.Model
	modelArg := ""
	if model != "" {
		modelArg = model
	}

	switch layout.ConfigFormat {
	case "toml_model":
		if prov != nil {
			return "", errs.New("%s: provider-backed model rendering is not supported for %s", profile.Path, layout.ConfigFormat)
		}
		if err := WriteToml(target, approvalSandboxConfigFor(modelArg, permission)); err != nil {
			return "", err
		}
	case "json_model":
		if err := contract.WriteJSON(target, buildClaudeConfig(modelArg, permission)); err != nil {
			return "", err
		}
	case "json_pi":
		if prov != nil && model != "" {
			om, _ := prov.Options.(map[string]any)
			upstream := getStr(om, "upstream_model")
			if upstream == "" {
				return "", errs.New("%s: provider-backed model %q must declare 'upstream_model' for pi", profile.Path, model)
			}
			if err := contract.WriteJSON(target, buildPiConfig(model, []string{piProviderID(model), upstream})); err != nil {
				return "", err
			}
			return relPath, nil
		}
		if err := contract.WriteJSON(target, buildPiConfig(modelArg, nil)); err != nil {
			return "", err
		}
	default:
		return "", errs.New("unsupported config_format %q; supported values: toml_model, json_model, json_pi", layout.ConfigFormat)
	}
	return relPath, nil
}

// --- main render ------------------------------------------------------------

// RenderLocalWorkspace projects one manifest profile into a local Agent workspace.
func RenderLocalWorkspace(repoRoot, profileName, outDir, manifestPath, modelOverride string) error {
	return RenderLocalWorkspaceWithOptions(repoRoot, profileName, outDir, manifestPath, ExecutionOptions{Model: modelOverride})
}

// ExecutionOptions are caller-owned settings; they are never written to source profiles.
type ExecutionOptions struct {
	Agent          string
	Model          string
	PermissionMode string
}

func RenderLocalWorkspaceWithOptions(repoRoot, profileName, outDir, manifestPath string, execution ExecutionOptions) error {
	if manifestPath == "" {
		manifestPath = filepath.Join(repoRoot, "dist", "agent-pack.manifest.json")
	}
	manifest, err := contract.LoadAgentPackManifest(manifestPath)
	if err != nil {
		return err
	}
	profileEntries, _ := manifest["profiles"].(map[string]any)
	assets, err := contract.LoadAssets(repoRoot)
	if err != nil {
		return err
	}

	profileEntryRaw, ok := profileEntries[profileName]
	if !ok {
		suggestion := strings.ReplaceAll(profileName, "_", "-")
		if _, has := profileEntries[suggestion]; has {
			return errs.New("unknown profile %q; did you mean %q?", profileName, suggestion)
		}
		known := make([]string, 0, len(profileEntries))
		for k := range profileEntries {
			known = append(known, k)
		}
		sort.Strings(known)
		return errs.New("unknown profile %q; known profiles: %s", profileName, strings.Join(known, ", "))
	}
	profileEntry, _ := profileEntryRaw.(map[string]any)
	profile, err := profileFromManifestEntry(repoRoot, profileName, profileEntry)
	if err != nil {
		return err
	}
	if agent := strings.TrimSpace(execution.Agent); agent != "" {
		profile.ProfileType = agent
	}
	if profile.ProfileType == "" {
		return errs.New("render v2 profile %q: --agent is required", profileName)
	}
	if mode := strings.TrimSpace(execution.PermissionMode); mode != "" {
		if !containsStr(contract.PermissionModes, mode) {
			return errs.New("render: permission mode must be default, plan or bypass")
		}
		resolved, err := resolveLocalAgent(profile.ProfileType)
		if err != nil {
			return err
		}
		if resolved == "pi" {
			return errs.New("render: agent %q does not support permission modes", resolved)
		}
		profile.Permission = &contract.Permission{Mode: mode}
	}
	profile, err = applyModelOverride(profile, execution.Model)
	if err != nil {
		return err
	}

	bindings := map[string]contract.Provider{}
	if manifest["schema_version"] == "v1.1" {
		providerEntries, _ := manifest["providers"].(map[string]any)
		for id, entryRaw := range providerEntries {
			entry, _ := entryRaw.(map[string]any)
			prov, err := providerFromManifest(entry, "manifest providers."+id)
			if err != nil {
				return err
			}
			bindings[id] = prov
		}
	}

	for _, f := range profile.InstructionFiles {
		if _, err := os.Stat(f); err != nil {
			return errs.New("%s: missing instructions file %s", manifestPath, f)
		}
	}
	for _, skillName := range profile.Skills {
		asset, ok := assets[skillName]
		if !ok || asset.AssetType != "skill" {
			return errs.New("%s: unknown skill %q", manifestPath, skillName)
		}
	}
	for _, subagentName := range profile.Subagents {
		asset, ok := assets[subagentName]
		if !ok || asset.AssetType != "subagent" {
			return errs.New("%s: unknown subagent %q", manifestPath, subagentName)
		}
	}

	resolvedAgent, err := resolveLocalAgent(profile.ProfileType)
	if err != nil {
		return err
	}
	layout, ok := LocalWorkspaceAgents[resolvedAgent]
	if !ok {
		keys := make([]string, 0, len(LocalWorkspaceAgents))
		for k := range LocalWorkspaceAgents {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return errs.New("unsupported local agent %q; supported values: %s", resolvedAgent, strings.Join(keys, ", "))
	}
	var selectedProvider *contract.Provider
	if manifest["schema_version"] == "v1.1" {
		selectedProvider, err = profileSelectedProvider(profile, bindings)
		if err != nil {
			return err
		}
	}

	cleanOut := filepath.Clean(outDir)
	if cleanOut == "." || cleanOut == string(filepath.Separator) {
		return errs.New("refusing to render into unsafe output dir %q", outDir)
	}
	if filepath.Clean(repoRoot) == cleanOut {
		return errs.New("refusing to render into repo root %q", outDir)
	}
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	if selectedProvider != nil && resolvedAgent == "pi" {
		regs, err := buildPiProviderRegistrations(profile, bindings)
		if err != nil {
			return err
		}
		extDir := filepath.Join(outDir, filepath.FromSlash(layout.ExtensionsDir))
		if err := os.MkdirAll(extDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(extDir, "agent-pack-provider.ts"), []byte(serializePiProviderExtension(regs)), 0o644); err != nil {
			return err
		}
	}

	instructionsContent, err := renderProfileInstructions(profile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, filepath.FromSlash(layout.InstructionsFile)), []byte(instructionsContent), 0o644); err != nil {
		return err
	}

	skillsDir := filepath.Join(outDir, filepath.FromSlash(layout.SkillsDir))
	for _, skillName := range profile.Skills {
		if err := copyDir(assets[skillName].Root, filepath.Join(skillsDir, skillName)); err != nil {
			return err
		}
	}

	subagentsDir := filepath.Join(outDir, filepath.FromSlash(layout.SubagentsDir))
	for _, subagentName := range profile.Subagents {
		if err := renderLocalSubagent(assets[subagentName], filepath.Join(subagentsDir, subagentName+".md")); err != nil {
			return err
		}
	}

	modelConfigFile, err := renderLocalAgentConfig(layout, profile, selectedProvider, outDir)
	if err != nil {
		return err
	}

	workspace := jsonx.NewMap()
	workspace.Set("agent", resolvedAgent)
	workspace.Set("profile", profile.Name)
	workspace.Set("type", profile.ProfileType)
	if profile.Model != "" {
		workspace.Set("model", profile.Model)
	}
	if profile.SupportedModels != nil {
		workspace.Set("supported_models", strList(profile.SupportedModels))
	}
	workspace.Set("instructions_file", layout.InstructionsFile)
	workspace.Set("skills_dir", layout.SkillsDir)
	workspace.Set("subagents_dir", layout.SubagentsDir)
	workspace.Set("model_config_file", modelConfigFile)
	workspace.Set("skills", strList(profile.Skills))
	workspace.Set("subagents", strList(profile.Subagents))
	if manifest["schema_version"] == "v1.1" {
		workspace.Set("providers", strList(profile.ProviderRefs))
	}
	if len(profile.Envs) > 0 {
		workspace.Set("envs", strList(profile.Envs))
	}
	if profile.Extends != "" {
		workspace.Set("extends", profile.Extends)
	}
	if profile.Permission != nil {
		perm := jsonx.NewMap()
		perm.Set("permission_mode", profile.Permission.Mode)
		perm.Set("allowed_tools", strList(profile.Permission.AllowedTools))
		perm.Set("ask_tools", strList(profile.Permission.AskTools))
		workspace.Set("permission", perm)
	}
	if selectedProvider != nil {
		provObj := jsonx.NewMap()
		provObj.Set("platform", selectedProvider.Platform)
		provObj.Set("api_key_env", selectedProvider.APIKeyEnv)
		om, _ := selectedProvider.Options.(map[string]any)
		if om != nil {
			for _, key := range []string{"base_url", "upstream_model"} {
				if v, ok := om[key]; ok {
					provObj.Set(key, v)
				}
			}
		}
		workspace.Set("provider", provObj)
		if resolvedAgent == "pi" && profile.Model != "" {
			om, _ := selectedProvider.Options.(map[string]any)
			upstream := getStr(om, "upstream_model")
			if upstream != "" {
				workspace.Set("launch_args", []any{"--model", piProviderID(profile.Model) + "/" + upstream})
			}
		}
	}

	if err := contract.WriteJSON(filepath.Join(outDir, "workspace-profile.json"), workspace); err != nil {
		return err
	}
	return nil
}

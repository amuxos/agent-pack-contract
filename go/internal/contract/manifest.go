package contract

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
	"github.com/amuxos/agent-pack-contract/go/internal/provider"
)

// --- small helpers ----------------------------------------------------------

func strList(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func hasKey(obj any, key string) bool {
	switch o := obj.(type) {
	case map[string]any:
		_, ok := o[key]
		return ok
	case map[string]Provider:
		_, ok := o[key]
		return ok
	case *jsonx.OrderedMap:
		_, ok := o.Get(key)
		return ok
	}
	return false
}

func toMap(obj any) (map[string]any, bool) {
	switch o := obj.(type) {
	case map[string]any:
		return o, true
	case *jsonx.OrderedMap:
		return jsonx.Plain(o).(map[string]any), true
	}
	return nil, false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// eachEntry iterates a map-like object (ordered map keeps insertion order,
// plain map iterates sorted for determinism).
func eachEntry(obj any, fn func(key string, val any) error) error {
	switch o := obj.(type) {
	case *jsonx.OrderedMap:
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			if err := fn(k, v); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, k := range sortedKeys(o) {
			if err := fn(k, o[k]); err != nil {
				return err
			}
		}
		return nil
	}
	return errs.New("expected object")
}

// --- source loaders ---------------------------------------------------------

func loadAgentPack(repoRoot string) (map[string]any, error) {
	path := filepath.Join(repoRoot, "agent-pack.toml")
	if _, err := os.Stat(path); err != nil {
		return nil, errs.New("missing agent-pack.toml")
	}
	data, err := loadToml(path)
	if err != nil {
		return nil, err
	}
	sv, err := requireString(data, "schema_version", "agent-pack.toml")
	if err != nil {
		return nil, err
	}
	if sv != "v1.1" && sv != "v2" {
		return nil, errs.New("agent-pack.toml: schema_version must be 'v1.1' or 'v2'")
	}
	data["schema_version"] = sv
	if sv == "v2" {
		if err := rejectPortableProviderFiles(repoRoot); err != nil {
			return nil, err
		}
	}
	if _, err := requireString(data, "name", "agent-pack.toml"); err != nil {
		return nil, err
	}
	if _, err := requireString(data, "namespace", "agent-pack.toml"); err != nil {
		return nil, err
	}
	if _, err := requireString(data, "description", "agent-pack.toml"); err != nil {
		return nil, err
	}
	if _, err := requireStringList(data, "owners", "agent-pack.toml"); err != nil {
		return nil, err
	}
	return data, nil
}

func parsePermission(data any, context string) (*Permission, error) {
	raw, present := getField(data, "permission")
	if !present || raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errs.New("%s: [permission] must be a table", context)
	}
	ctx := context + " [permission]"
	mode, err := optionalString(m, "permission_mode", ctx, PermissionModes...)
	if err != nil {
		return nil, err
	}
	allowed, err := requireStringList(m, "allowed_tools", ctx)
	if err != nil {
		return nil, err
	}
	ask, err := requireStringList(m, "ask_tools", ctx)
	if err != nil {
		return nil, err
	}
	return &Permission{Mode: mode, AllowedTools: allowed, AskTools: ask}, nil
}

func parseProviderTable(raw *jsonx.OrderedMap, context string) (Provider, error) {
	for _, forbidden := range ForbiddenProviderKeys {
		if _, ok := raw.Get(forbidden); ok {
			return Provider{}, errs.New(
				"%s: inline credential field %q is not allowed; store only the env var name in 'api_key_env' (rendered as ${NAME})",
				context, forbidden)
		}
	}
	platform, err := requireString(raw, "platform", context)
	if err != nil {
		return Provider{}, err
	}
	if !contains(provider.SupportedPlatforms, platform) {
		return Provider{}, errs.New("%s: unknown platform %q; supported values: %s",
			context, platform, strings.Join(provider.SupportedPlatforms, ", "))
	}
	apiKeyEnv, err := requireString(raw, "api_key_env", context)
	if err != nil {
		return Provider{}, err
	}
	if !ENVNameRE.MatchString(apiKeyEnv) {
		return Provider{}, errs.New(
			"%s: 'api_key_env' must be a bare environment variable name (matching %s); got %q",
			context, ENVNameRE.String(), apiKeyEnv)
	}
	options := jsonx.NewMap()
	for _, k := range raw.Keys() {
		if k == "platform" || k == "api_key_env" {
			continue
		}
		v, _ := raw.Get(k)
		options.Set(k, v)
	}
	return Provider{Platform: platform, APIKeyEnv: apiKeyEnv, Options: options}, nil
}

func providerToManifest(p Provider) *jsonx.OrderedMap {
	m := jsonx.NewMap()
	m.Set("platform", p.Platform)
	m.Set("api_key_env", p.APIKeyEnv)
	switch o := p.Options.(type) {
	case *jsonx.OrderedMap:
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			m.Set(k, v)
		}
	case map[string]any:
		for _, k := range sortedKeys(o) {
			m.Set(k, o[k])
		}
	}
	return m
}

type profileFile struct {
	path string
	data map[string]any
}

func discoverProfileFiles(repoRoot string) (map[string]profileFile, error) {
	profilesDir := filepath.Join(repoRoot, "profiles")
	flat, _ := filepath.Glob(filepath.Join(profilesDir, "*.toml"))
	if len(flat) > 0 {
		var names []string
		for _, p := range flat {
			names = append(names, filepath.Base(p))
		}
		sort.Strings(names)
		return nil, errs.New("profiles/: v1.1 profiles must live under profiles/<namespace>/<name>.toml; found %s",
			strings.Join(names, ", "))
	}
	paths, _ := filepath.Glob(filepath.Join(profilesDir, "*", "*.toml"))
	sort.Strings(paths)
	raw := map[string]profileFile{}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if _, ok := raw[name]; ok {
			return nil, errs.New("profiles/: duplicate profile ID %q", name)
		}
		data, err := loadToml(path)
		if err != nil {
			return nil, err
		}
		raw[name] = profileFile{path: path, data: data}
	}
	return raw, nil
}

func profileNamespace(repoRoot, name, path string) (string, error) {
	profilesDir := filepath.Join(repoRoot, "profiles")
	if filepath.Dir(filepath.Dir(path)) == profilesDir {
		ns := filepath.Base(filepath.Dir(path))
		if ns == "" {
			return "", errs.New("%s: empty namespace", path)
		}
		return ns, nil
	}
	return "", errs.New("%s: profile must live under profiles/<namespace>/<name>.toml", path)
}

func resolveExtends(name, path string, data map[string]any, raw map[string]profileFile) (string, map[string]any, error) {
	extendsVal, present := data["extends"]
	if !present || extendsVal == nil {
		return "", data, nil
	}
	extends, ok := extendsVal.(string)
	if !ok || strings.TrimSpace(extends) == "" {
		return "", nil, errs.New("%s: 'extends' must be a non-empty profile name", filepath.Base(path))
	}
	extends = strings.TrimSpace(extends)
	if extends == name {
		return "", nil, errs.New("%s: 'extends' cannot reference itself", filepath.Base(path))
	}
	base, ok := raw[extends]
	if !ok {
		known := make([]string, 0, len(raw))
		for k := range raw {
			known = append(known, k)
		}
		sort.Strings(known)
		return "", nil, errs.New("%s: extends unknown profile %q; known: %s",
			filepath.Base(path), extends, strings.Join(known, ", "))
	}
	if _, hasExt := base.data["extends"]; hasExt {
		return "", nil, errs.New("%s: extends target %q must not itself use 'extends' (chains unsupported)",
			filepath.Base(path), extends)
	}
	merged := map[string]any{}
	for k, v := range base.data {
		merged[k] = v
	}
	for k, v := range data {
		if k != "extends" {
			merged[k] = v
		}
	}
	return extends, merged, nil
}

func isInside(repoRoot, path string) bool {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func loadProfiles(repoRoot string) (map[string]*Profile, error) {
	pack, err := loadAgentPack(repoRoot)
	if err != nil {
		return nil, err
	}
	portable := pack["schema_version"] == "v2"
	raw, err := discoverProfileFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	profiles := map[string]*Profile{}
	for _, name := range sortedProfileNames(raw) {
		pf := raw[name]
		extends, merged, err := resolveExtends(name, pf.path, pf.data, raw)
		if err != nil {
			return nil, err
		}
		namespace, err := profileNamespace(repoRoot, name, pf.path)
		if err != nil {
			return nil, err
		}
		instructionValues, err := requireStringList(merged, "instructions", filepath.Base(pf.path))
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
		if portable {
			if err := rejectProfileExecutionFields(merged, filepath.Base(pf.path)); err != nil {
				return nil, err
			}
		}
		profileType, err := optionalString(merged, "type", filepath.Base(pf.path))
		if !portable {
			profileType, err = requireString(merged, "type", filepath.Base(pf.path))
		}
		if err != nil {
			return nil, err
		}
		if _, present := getField(merged, "provider"); present {
			return nil, errs.New("%s: inline [provider] tables are not supported in v1.1; use providers/<model-ref>.toml",
				filepath.Base(pf.path))
		}
		providerRefs, err := requireStringList(merged, "providers", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		permission, err := parsePermission(merged, filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		description, err := optionalString(merged, "description", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		model, err := optionalString(merged, "model", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		supported, err := optionalStringList(merged, "supported_models", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		profile := &Profile{
			Name:             name,
			Namespace:        namespace,
			ProfileType:      profileType,
			Description:      description,
			Model:            model,
			Instructions:     instructions,
			InstructionFiles: instructionFiles,
			Skills:           nil,
			Subagents:        nil,
			Path:             pf.path,
			SupportedModels:  supported,
			ProviderRefs:     providerRefs,
			Envs:             nil,
			Permission:       permission,
			Extends:          extends,
		}
		profile.Skills, err = requireStringList(merged, "skills", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		profile.Subagents, err = requireStringList(merged, "subagents", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}
		profile.Envs, err = requireStringList(merged, "envs", filepath.Base(pf.path))
		if err != nil {
			return nil, err
		}

		if profile.Instructions == "" {
			// no instructions: ok
		} else if extends == "" {
			expected := filepath.Join(repoRoot, "instructions", namespace, name, "AGENTS.md")
			if profile.Instructions != expected {
				return nil, errs.New("%s: instructions must point to instructions/%s/%s/AGENTS.md",
					filepath.Base(pf.path), namespace, name)
			}
		} else {
			rel, _ := filepath.Rel(repoRoot, profile.Instructions)
			if filepath.Base(profile.Instructions) != "AGENTS.md" ||
				!strings.HasPrefix(filepath.ToSlash(rel), "instructions/") {
				return nil, errs.New("%s: instructions must point to instructions/<dir>/AGENTS.md",
					filepath.Base(pf.path))
			}
		}
		for _, f := range profile.InstructionFiles {
			if filepath.IsAbs(f) && !isInside(repoRoot, f) {
				return nil, errs.New("%s: instructions path must stay inside repo", filepath.Base(pf.path))
			}
			if _, err := os.Stat(f); err != nil {
				return nil, errs.New("%s: missing instructions file %s", filepath.Base(pf.path), f)
			}
		}
		profiles[name] = profile
	}
	return profiles, nil
}

func sortedProfileNames(raw map[string]profileFile) []string {
	names := make([]string, 0, len(raw))
	for k := range raw {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func assetRoot(repoRoot, assetType, sourceKind string) string {
	if assetType == "skill" {
		return filepath.Join(repoRoot, "skills", sourceKind)
	}
	return filepath.Join(repoRoot, "subagents", sourceKind)
}

func assetContentFilename(assetType string) string {
	if assetType == "skill" {
		return "SKILL.md"
	}
	return "AGENTS.md"
}

func loadAssets(repoRoot string) (map[string]*Asset, error) {
	assets := map[string]*Asset{}
	for _, assetType := range []string{"skill", "subagent"} {
		for _, sourceKind := range []string{"upstream", "custom"} {
			root := assetRoot(repoRoot, assetType, sourceKind)
			entries, err := os.ReadDir(root)
			if err != nil {
				continue // dir does not exist
			}
			var dirs []string
			for _, e := range entries {
				if e.IsDir() {
					dirs = append(dirs, e.Name())
				}
			}
			sort.Strings(dirs)
			for _, name := range dirs {
				if _, dup := assets[name]; dup {
					return nil, errs.New("asset name %q is duplicated across repo", name)
				}
				assetDir := filepath.Join(root, name)
				sourceToml := filepath.Join(assetDir, "source.toml")
				if _, err := os.Stat(sourceToml); err != nil {
					return nil, errs.New("%s: missing source.toml", assetDir)
				}
				data, err := loadToml(sourceToml)
				if err != nil {
					return nil, err
				}
				actualSourceKind, err := requireString(data, "source_kind", sourceToml)
				if err != nil {
					return nil, err
				}
				actualAssetType, err := requireString(data, "asset_type", sourceToml)
				if err != nil {
					return nil, err
				}
				if actualSourceKind != sourceKind {
					return nil, errs.New("%s: source_kind must be %q", sourceToml, sourceKind)
				}
				if actualAssetType != assetType {
					return nil, errs.New("%s: asset_type must be %q", sourceToml, assetType)
				}
				originVal, ok := data["origin"]
				if !ok {
					return nil, errs.New("%s: missing [origin] table", sourceToml)
				}
				origin, ok := originVal.(map[string]any)
				if !ok {
					return nil, errs.New("%s: missing [origin] table", sourceToml)
				}
				if _, err := requireString(origin, "kind", sourceToml); err != nil {
					return nil, err
				}
				contentFile := filepath.Join(assetDir, assetContentFilename(assetType))
				if _, err := os.Stat(contentFile); err != nil {
					return nil, errs.New("%s: missing %s", assetDir, assetContentFilename(assetType))
				}
				assets[name] = &Asset{
					Name:        name,
					AssetType:   assetType,
					SourceKind:  sourceKind,
					Root:        assetDir,
					ContentFile: contentFile,
					SourceToml:  sourceToml,
					Origin:      origin,
				}
			}
		}
	}
	return assets, nil
}

func validateContract(repoRoot string) ([]string, error) {
	if _, err := loadAgentPack(repoRoot); err != nil {
		return nil, err
	}
	profiles, err := loadProfiles(repoRoot)
	if err != nil {
		return nil, err
	}
	assets, err := loadAssets(repoRoot)
	if err != nil {
		return nil, err
	}

	var warnings []string
	usedAssets := map[string]bool{}
	for _, profile := range profiles {
		for _, skillName := range profile.Skills {
			asset, ok := assets[skillName]
			if !ok {
				return nil, errs.New("%s: unknown skill %q", filepath.Base(profile.Path), skillName)
			}
			if asset.AssetType != "skill" {
				return nil, errs.New("%s: %q is not a skill", filepath.Base(profile.Path), skillName)
			}
			usedAssets[skillName] = true
		}
		for _, subagentName := range profile.Subagents {
			asset, ok := assets[subagentName]
			if !ok {
				return nil, errs.New("%s: unknown subagent %q", filepath.Base(profile.Path), subagentName)
			}
			if asset.AssetType != "subagent" {
				return nil, errs.New("%s: %q is not a subagent", filepath.Base(profile.Path), subagentName)
			}
			usedAssets[subagentName] = true
		}
	}
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !usedAssets[name] {
			warnings = append(warnings, "warning: "+assets[name].AssetType+" "+name+" is not referenced by any profile")
		}
	}
	return warnings, nil
}

// --- provider bindings ------------------------------------------------------

// v2 cannot silently ignore leftover execution configuration during migration.
func rejectPortableProviderFiles(repoRoot string) error {
	root := filepath.Join(repoRoot, "providers")
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".toml") {
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			return errs.New("%s: provider files are not allowed in v2; configure models in the execution environment", filepath.ToSlash(rel))
		}
		return nil
	})
}

func loadProviderFiles(repoRoot string) (map[string]Provider, error) {
	providersDir := filepath.Join(repoRoot, "providers")
	if _, err := os.Stat(providersDir); err != nil {
		return map[string]Provider{}, nil
	}
	var paths []string
	err := filepath.WalkDir(providersDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".toml") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	providers := map[string]Provider{}
	for _, p := range paths {
		rel, err := filepath.Rel(providersDir, p)
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(filepath.ToSlash(rel), ".toml")
		if strings.Count(id, "/") > 1 {
			return nil, errs.New("%s: provider ref must use <model> or <provider>/<model>", p)
		}
		if _, dup := providers[id]; dup {
			return nil, errs.New("providers/: duplicate provider ID %q", id)
		}
		om, err := loadTomlOrdered(p)
		if err != nil {
			return nil, err
		}
		prov, err := parseProviderTable(om, p)
		if err != nil {
			return nil, err
		}
		providers[id] = prov
	}
	return providers, nil
}

func providerEqual(a, b Provider) bool {
	if a.Platform != b.Platform || a.APIKeyEnv != b.APIKeyEnv {
		return false
	}
	return reflect.DeepEqual(jsonx.Plain(a.Options), jsonx.Plain(b.Options))
}

func qualifiedProviderRef(ref, context string) (bool, error) {
	if !strings.Contains(ref, "/") {
		return false, nil
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return false, errs.New("%s: provider-backed model ref %q must use <provider>/<model>", context, ref)
	}
	return true, nil
}

func validateModelSelectionFields(model string, supported []string, providerRefs []string, providers any, context string) error {
	if supported == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, s := range supported {
		if seen[s] {
			return errs.New("%s: 'supported_models' must not contain duplicates", context)
		}
		seen[s] = true
	}
	if model != "" && !contains(supported, model) {
		return errs.New("%s: model %q must be listed in 'supported_models'", context, model)
	}
	if model == "" && len(providerRefs) > 0 {
		return errs.New("%s: model is required when 'providers' are declared; leave providers empty to follow the agent default", context)
	}
	for _, ref := range supported {
		q, err := qualifiedProviderRef(ref, context)
		if err != nil {
			return err
		}
		if q && !hasKey(providers, ref) {
			return errs.New("%s: unknown provider-backed model %q in 'supported_models'", context, ref)
		}
	}
	if model != "" {
		q, err := qualifiedProviderRef(model, context)
		if err != nil {
			return err
		}
		if q && !contains(providerRefs, model) {
			return errs.New("%s: provider-backed default model %q must also be listed in 'providers' for v1.1 compatibility", context, model)
		}
	}
	return nil
}

func loadProviderBindings(repoRoot string, profiles map[string]*Profile) (map[string]Provider, error) {
	bindings := map[string]Provider{}
	files, err := loadProviderFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	for id, prov := range files {
		existing, ok := bindings[id]
		if ok && !providerEqual(existing, prov) {
			return nil, errs.New("providers/%s.toml: provider-backed model %q has conflicting config", id, id)
		}
		bindings[id] = prov
	}
	for _, profile := range profiles {
		for _, ref := range profile.ProviderRefs {
			if !hasKey(bindings, ref) {
				return nil, errs.New("%s: unknown provider-backed model %q", filepath.Base(profile.Path), ref)
			}
		}
		if err := validateModelSelectionFields(profile.Model, profile.SupportedModels, profile.ProviderRefs, bindings, filepath.Base(profile.Path)); err != nil {
			return nil, err
		}
	}
	return bindings, nil
}

// --- permission / path helpers ---------------------------------------------

func permissionToManifest(p *Permission) *jsonx.OrderedMap {
	if p == nil {
		return nil
	}
	m := jsonx.NewMap()
	if p.Mode != "" {
		m.Set("permission_mode", p.Mode)
	}
	if len(p.AllowedTools) > 0 {
		m.Set("allowed_tools", strList(p.AllowedTools))
	}
	if len(p.AskTools) > 0 {
		m.Set("ask_tools", strList(p.AskTools))
	}
	return m
}

func relativePath(repoRoot, path string) (string, error) {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errs.New("%s: path must stay inside repo", path)
	}
	return filepath.ToSlash(rel), nil
}

// --- manifest build ---------------------------------------------------------

// BuildAgentPackManifest builds the canonical manifest as an ordered map so
// JSON bytes stay stable.
func BuildAgentPackManifest(repoRoot string) (*jsonx.OrderedMap, error) {
	pack, err := loadAgentPack(repoRoot)
	if err != nil {
		return nil, err
	}
	profiles, err := loadProfiles(repoRoot)
	if err != nil {
		return nil, err
	}
	providerBindings := map[string]Provider{}
	if pack["schema_version"] == "v1.1" {
		providerBindings, err = loadProviderBindings(repoRoot, profiles)
		if err != nil {
			return nil, err
		}
	}

	providerPayloads := map[string]*jsonx.OrderedMap{}
	for id, prov := range providerBindings {
		providerPayloads[id] = providerToManifest(prov)
	}

	profilePayloads := map[string]*jsonx.OrderedMap{}
	namespaceIndex := map[string][]string{}
	profileNames := make([]string, 0, len(profiles))
	for name := range profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	for _, name := range profileNames {
		profile := profiles[name]
		for _, ref := range profile.ProviderRefs {
			if _, ok := providerPayloads[ref]; !ok {
				return nil, errs.New("%s: unknown provider-backed model %q", filepath.Base(profile.Path), ref)
			}
		}
		entry := jsonx.NewMap()
		entry.Set("namespace", profile.Namespace)
		if pack["schema_version"] != "v2" {
			entry.Set("type", profile.ProfileType)
		}
		if profile.Description != "" {
			entry.Set("description", profile.Description)
		}
		if profile.Model != "" {
			entry.Set("model", profile.Model)
		}
		if profile.SupportedModels != nil {
			entry.Set("supported_models", strList(profile.SupportedModels))
		}
		var relInstructions []string
		for _, f := range profile.InstructionFiles {
			rel, err := relativePath(repoRoot, f)
			if err != nil {
				return nil, err
			}
			relInstructions = append(relInstructions, rel)
		}
		if pack["schema_version"] != "v2" {
			entry.Set("providers", strList(profile.ProviderRefs))
		}
		entry.Set("instructions", strList(relInstructions))
		entry.Set("skills", strList(profile.Skills))
		entry.Set("subagents", strList(profile.Subagents))
		if profile.Extends == "" {
			entry.Set("extends", nil)
		} else {
			entry.Set("extends", profile.Extends)
		}
		if len(profile.Envs) > 0 {
			entry.Set("envs", strList(profile.Envs))
		}
		if perm := permissionToManifest(profile.Permission); perm != nil {
			entry.Set("permission", perm)
		}
		profilePayloads[name] = entry
		namespaceIndex[profile.Namespace] = append(namespaceIndex[profile.Namespace], name)
	}

	orderedNamespaces := jsonx.NewMap()
	nsNames := make([]string, 0, len(namespaceIndex))
	for ns := range namespaceIndex {
		nsNames = append(nsNames, ns)
	}
	sort.Strings(nsNames)
	for _, ns := range nsNames {
		names := namespaceIndex[ns]
		sort.Strings(names)
		e := jsonx.NewMap()
		e.Set("profiles", strList(names))
		orderedNamespaces.Set(ns, e)
	}

	orderedProfiles := jsonx.NewMap()
	for _, name := range profileNames {
		orderedProfiles.Set(name, profilePayloads[name])
	}
	orderedProviders := jsonx.NewMap()
	provIDs := make([]string, 0, len(providerPayloads))
	for id := range providerPayloads {
		provIDs = append(provIDs, id)
	}
	sort.Strings(provIDs)
	for _, id := range provIDs {
		orderedProviders.Set(id, providerPayloads[id])
	}

	manifest := jsonx.NewMap()
	manifest.Set("schema_version", pack["schema_version"])
	manifest.Set("pack_namespace", pack["namespace"])
	manifest.Set("profiles", orderedProfiles)
	if pack["schema_version"] == "v1.1" {
		manifest.Set("providers", orderedProviders)
	}
	manifest.Set("namespaces", orderedNamespaces)

	if err := ValidateAgentPackManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// --- manifest validation ----------------------------------------------------

// ValidateAgentPackManifest checks cross-field invariants that the JSON Schema
// cannot express.
func ValidateAgentPackManifest(manifest any) error {
	sv, _ := getField(manifest, "schema_version")
	if sv != "v1.1" && sv != "v2" {
		return errs.New("manifest: schema_version must be 'v1.1' or 'v2'")
	}
	if _, err := requireString(manifest, "pack_namespace", "manifest"); err != nil {
		return err
	}
	profilesRaw, _ := getField(manifest, "profiles")
	if _, ok := toMap(profilesRaw); !ok {
		return errs.New("manifest: profiles must be a map")
	}
	providersRaw, providersPresent := getField(manifest, "providers")
	if sv == "v2" {
		if providersPresent {
			return errs.New("manifest: top-level providers is not allowed in v2; configure models in the execution environment")
		}
		providersRaw = map[string]any{}
	} else if _, ok := toMap(providersRaw); !ok {
		return errs.New("manifest: providers must be a map")
	}
	namespacesRaw, _ := getField(manifest, "namespaces")
	nsMap, ok := toMap(namespacesRaw)
	if !ok {
		return errs.New("manifest: namespaces must be a map")
	}

	namespaceIndex := map[string][]string{}
	if err := eachEntry(profilesRaw, func(profileID string, entryVal any) error {
		if profileID == "" {
			return errs.New("manifest: profile IDs must be non-empty strings")
		}
		entry, ok := toMap(entryVal)
		if !ok {
			return errs.New("manifest profiles.%s: must be an object", profileID)
		}
		namespace, err := requireString(entry, "namespace", "manifest profiles."+profileID)
		if err != nil {
			return err
		}
		if sv == "v2" {
			if err := rejectProfileExecutionFields(entry, "manifest profiles."+profileID); err != nil {
				return err
			}
		} else if _, err := requireString(entry, "type", "manifest profiles."+profileID); err != nil {
			return err
		}
		model, err := optionalString(entry, "model", "manifest profiles."+profileID)
		if err != nil {
			return err
		}
		providerRefs, err := requireStringList(entry, "providers", "manifest profiles."+profileID)
		if err != nil {
			return err
		}
		for _, ref := range providerRefs {
			if !hasKey(providersRaw, ref) {
				return errs.New("manifest profiles.%s: unknown provider-backed model %q", profileID, ref)
			}
		}
		if model != "" && contains(providerRefs, model) && !hasKey(providersRaw, model) {
			return errs.New("manifest profiles.%s: model provider config missing", profileID)
		}
		supported, err := optionalStringList(entry, "supported_models", "manifest profiles."+profileID)
		if err != nil {
			return err
		}
		if err := validateModelSelectionFields(model, supported, providerRefs, providersRaw, "manifest profiles."+profileID); err != nil {
			return err
		}
		instructionsRaw, present := getField(entry, "instructions")
		var instructions []any
		if !present {
			// A missing instructions field defaults to an empty list.
			instructions = []any{}
		} else {
			arr, ok := instructionsRaw.([]any)
			if !ok {
				return errs.New("manifest profiles.%s: instructions must be an array", profileID)
			}
			instructions = arr
		}
		for _, item := range instructions {
			if s, ok := item.(string); !ok || strings.TrimSpace(s) == "" {
				return errs.New("manifest profiles.%s: instructions must contain strings", profileID)
			}
		}
		if _, err := requireStringList(entry, "skills", "manifest profiles."+profileID); err != nil {
			return err
		}
		if _, err := requireStringList(entry, "subagents", "manifest profiles."+profileID); err != nil {
			return err
		}
		if _, err := requireStringList(entry, "envs", "manifest profiles."+profileID); err != nil {
			return err
		}
		if v, present := getField(entry, "extends"); present && v != nil {
			if _, ok := v.(string); !ok {
				return errs.New("manifest profiles.%s: extends must be string or null", profileID)
			}
		}
		if v, present := getField(entry, "permission"); present && v != nil {
			if _, ok := toMap(v); !ok {
				return errs.New("manifest profiles.%s: permission must be an object", profileID)
			}
		}
		namespaceIndex[namespace] = append(namespaceIndex[namespace], profileID)
		return nil
	}); err != nil {
		return err
	}

	if err := eachEntry(providersRaw, func(providerID string, entryVal any) error {
		if providerID == "" {
			return errs.New("manifest providers: provider IDs must be non-empty strings")
		}
		entry, ok := toMap(entryVal)
		if !ok {
			return errs.New("manifest providers.%s: must be an object", providerID)
		}
		if _, err := requireString(entry, "platform", "manifest providers."+providerID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := eachEntry(namespacesRaw, func(namespace string, entryVal any) error {
		entry, ok := toMap(entryVal)
		if !ok {
			return errs.New("manifest namespaces.%s: must be an object", namespace)
		}
		listed, err := requireStringList(entry, "profiles", "manifest namespaces."+namespace)
		if err != nil {
			return err
		}
		expected := append([]string{}, namespaceIndex[namespace]...)
		sort.Strings(expected)
		sort.Strings(listed)
		if !reflect.DeepEqual(listed, expected) {
			return errs.New("manifest namespaces.%s: profiles must match profiles.*.namespace", namespace)
		}
		return nil
	}); err != nil {
		return err
	}

	for ns := range nsMap {
		if _, ok := namespaceIndex[ns]; !ok {
			return errs.New("manifest namespaces: unknown namespace(s) %s", ns)
		}
	}
	return nil
}

// LoadAgentPackManifest reads and validates a generated manifest.
func LoadAgentPackManifest(path string) (map[string]any, error) {
	manifest, err := ReadJSON(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateAgentPackManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// v2 separates authored content from execution choices. Presence is forbidden,
// even for an empty value, so a partial migration cannot silently change behavior.
func rejectProfileExecutionFields(entry any, context string) error {
	for _, key := range []string{"type", "model", "providers", "supported_models", "permission"} {
		if hasKey(entry, key) {
			return errs.New("%s: field %q is not allowed in v2 profiles; supply execution settings when running or rendering", context, key)
		}
	}
	return nil
}

// Package provider provides per-platform adapters that
// build standard pi model entries.
package provider

import (
	"sort"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
)

type piAdapter func(options any, apiKeyEnv, ref string) (*jsonx.OrderedMap, *jsonx.OrderedMap, error)

type platformAdapter struct {
	Pi piAdapter
}

// platformAdapters is the single registry for supported provider platforms.
// Adding a platform must provide its standard Pi projection in one place.
var platformAdapters = map[string]platformAdapter{
	"openai-compat": {Pi: openaiCompatPiEntry},
}

// SupportedPlatforms lists registered provider platforms in stable order.
var SupportedPlatforms = sortedPlatformNames()

func sortedPlatformNames() []string {
	names := make([]string, 0, len(platformAdapters))
	for name := range platformAdapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// --- option readers ---------------------------------------------------------

func optionGet(options any, key string) (any, bool) {
	switch o := options.(type) {
	case map[string]any:
		v, ok := o[key]
		return v, ok
	case *jsonx.OrderedMap:
		return o.Get(key)
	}
	return nil, false
}

func OptionStr(options any, key, context string, required bool, def string) (string, error) {
	v, present := optionGet(options, key)
	if !present {
		if required {
			return "", errs.New("%s: missing required option %q", context, key)
		}
		return def, nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", errs.New("%s: option %q must be a non-empty string", context, key)
	}
	return strings.TrimSpace(s), nil
}

func OptionBool(options any, key, context string, def bool) (bool, error) {
	v, present := optionGet(options, key)
	if !present {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, errs.New("%s: option %q must be a boolean", context, key)
	}
	return b, nil
}

func OptionInt(options any, key, context string) (*int64, error) {
	v, present := optionGet(options, key)
	if !present {
		return nil, nil
	}
	switch n := v.(type) {
	case int64:
		x := n
		return &x, nil
	case int:
		x := int64(n)
		return &x, nil
	case float64: // numbers read back from manifest JSON
		if n != float64(int64(n)) {
			return nil, errs.New("%s: option %q must be an integer", context, key)
		}
		x := int64(n)
		return &x, nil
	}
	return nil, errs.New("%s: option %q must be an integer", context, key)
}

func OptionStrMap(options any, key, context string) (map[string]string, error) {
	v, present := optionGet(options, key)
	if !present {
		return map[string]string{}, nil
	}
	switch m := v.(type) {
	case map[string]any:
		out := make(map[string]string, len(m))
		for k, val := range m {
			s, ok := val.(string)
			if !ok {
				return nil, errs.New("%s: option %q must be a table of string values", context, key)
			}
			out[k] = s
		}
		return out, nil
	case *jsonx.OrderedMap:
		out := make(map[string]string, m.Len())
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			s, ok := val.(string)
			if !ok {
				return nil, errs.New("%s: option %q must be a table of string values", context, key)
			}
			out[k] = s
		}
		return out, nil
	}
	return nil, errs.New("%s: option %q must be a table of string values", context, key)
}

func optionKeys(options any) []string {
	switch o := options.(type) {
	case *jsonx.OrderedMap:
		return o.Keys()
	case map[string]any:
		keys := make([]string, 0, len(o))
		for k := range o {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	return nil
}

func rejectUnknownOptions(options any, accepted []string, context string) error {
	acceptedSet := make(map[string]bool, len(accepted))
	for _, a := range accepted {
		acceptedSet[a] = true
	}
	var unknown []string
	for _, k := range optionKeys(options) {
		if !acceptedSet[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		sort.Strings(accepted)
		return errs.New("%s: unknown option(s) %s; accepted: %s", context,
			strings.Join(unknown, ", "), strings.Join(accepted, ", "))
	}
	return nil
}

// --- pi adapters ------------------------------------------------------------

func piModelEntry(id, name string, reasoning bool, contextWindow, maxTokens int64, compat *jsonx.OrderedMap) *jsonx.OrderedMap {
	m := jsonx.NewMap()
	m.Set("id", id)
	m.Set("name", name)
	m.Set("reasoning", reasoning)
	m.Set("input", []any{"text"})
	cost := jsonx.NewMap()
	cost.Set("input", int64(0))
	cost.Set("output", int64(0))
	cost.Set("cacheRead", int64(0))
	cost.Set("cacheWrite", int64(0))
	m.Set("cost", cost)
	m.Set("contextWindow", contextWindow)
	m.Set("maxTokens", maxTokens)
	if compat != nil {
		m.Set("compat", compat)
	}
	return m
}

func openaiCompatPiEntry(options any, apiKeyEnv, ref string) (*jsonx.OrderedMap, *jsonx.OrderedMap, error) {
	context := "openai-compat provider"
	accepted := []string{"base_url", "upstream_model", "context_window", "max_tokens", "disable_failover", "thinking"}
	if err := rejectUnknownOptions(options, accepted, context); err != nil {
		return nil, nil, err
	}
	baseURL, err := OptionStr(options, "base_url", context, true, "")
	if err != nil {
		return nil, nil, err
	}
	upstreamModel, err := OptionStr(options, "upstream_model", context, false, "default_model")
	if err != nil {
		return nil, nil, err
	}
	contextWindow, err := OptionInt(options, "context_window", context)
	if err != nil {
		return nil, nil, err
	}
	maxTokens, err := OptionInt(options, "max_tokens", context)
	if err != nil {
		return nil, nil, err
	}
	if maxTokens != nil && *maxTokens <= 0 {
		return nil, nil, errs.New("%s: option 'max_tokens' must be positive", context)
	}
	thinking, err := OptionStrMap(options, "thinking", context)
	if err != nil {
		return nil, nil, err
	}

	cw := int64(128000)
	if contextWindow != nil {
		cw = *contextWindow
	}
	mt := int64(16384)
	if maxTokens != nil {
		mt = *maxTokens
	}
	modelEntry := piModelEntry(upstreamModel, ref, len(thinking) > 0, cw, mt, nil)

	connection := jsonx.NewMap()
	connection.Set("baseUrl", baseURL)
	connection.Set("api", "openai-completions")
	return modelEntry, connection, nil
}

// BuildPiProviderEntry returns a model entry and a connection carrying the
// provider-wide fields that must agree across refs sharing the same pi
// provider id.
func BuildPiProviderEntry(platform string, options any, apiKeyEnv, ref string) (*jsonx.OrderedMap, *jsonx.OrderedMap, error) {
	adapter, ok := platformAdapters[platform]
	if !ok {
		return nil, nil, errs.New("unknown provider platform %q; supported values: %s", platform, strings.Join(SupportedPlatforms, ", "))
	}
	return adapter.Pi(options, apiKeyEnv, ref)
}

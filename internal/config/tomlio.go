package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/atomicfile"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	toml "github.com/pelletier/go-toml/v2"
)

// Load reads and parses a TOML config file into a generic map.
// Missing file returns an empty map (not an error).
func Load(configPath string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]interface{}{}, nil
	}
	var data map[string]interface{}
	if err := toml.Unmarshal(raw, &data); err != nil {
		return nil, &exitcodes.ConfigParseError{Path: configPath, Err: err}
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	return data, nil
}

// MustLoad parses config; missing file is OK (empty map), parse errors are fatal.
func MustLoad(configPath string) (map[string]interface{}, error) {
	return Load(configPath)
}

// Apply writes Profile fields into config.toml, preserving unknown top-level sections.
func Apply(configPath string, p profiles.Profile) error {
	data, err := Load(configPath)
	if err != nil {
		return err
	}
	applyProfile(data, p)
	return atomicWriteTOML(configPath, data)
}

// applyProfile mutates data in-place with profile values.
func applyProfile(data map[string]interface{}, p profiles.Profile) {
	// endpoints
	data["endpoints"] = map[string]interface{}{
		"api_url": p.BaseURL,
	}

	// models
	data["models"] = map[string]interface{}{
		"default":    p.DefaultModel,
		"web_search": nonEmpty(p.WebSearchModel, p.DefaultModel),
	}

	// subagents.models
	explore := nonEmpty(p.SubagentsModels.Explore, p.DefaultModel)
	plan := nonEmpty(p.SubagentsModels.Plan, p.DefaultModel)
	data["subagents"] = map[string]interface{}{
		"models": map[string]interface{}{
			"explore": explore,
			"plan":    plan,
		},
	}

	// Remove all existing model.* sections, then write new ones.
	// pelletier/go-toml represents [model.xxx] as nested maps under "model".
	// Also handle flat keys if present.
	delete(data, "model")

	// Build model definitions.
	modelMap := map[string]interface{}{}

	// Primary default model section.
	effort := nonEmpty(p.DefaultReasoningEffort, "high")
	modelMap[p.DefaultModel] = map[string]interface{}{
		"model":                      p.DefaultModel,
		"api_key":                    p.APIKey,
		"supports_reasoning_effort":  true,
		"reasoning_effort":           effort,
		"reasoning_efforts":          []interface{}{"low", "medium", "high"},
	}

	// Additional models from profile.Models.
	for _, m := range p.Models {
		id := m.ID
		if id == "" {
			id = m.Model
		}
		if id == "" {
			continue
		}
		entry := map[string]interface{}{
			"model":                     nonEmpty(m.Model, id),
			"supports_reasoning_effort": m.SupportsReasoningEffort,
		}
		key := m.APIKey
		if key == "" {
			key = p.APIKey
		}
		entry["api_key"] = key
		if m.ReasoningEffort != "" {
			entry["reasoning_effort"] = m.ReasoningEffort
		} else {
			entry["reasoning_effort"] = effort
		}
		if len(m.ReasoningEfforts) > 0 {
			arr := make([]interface{}, len(m.ReasoningEfforts))
			for i, v := range m.ReasoningEfforts {
				arr[i] = v
			}
			entry["reasoning_efforts"] = arr
		} else {
			entry["reasoning_efforts"] = []interface{}{"low", "medium", "high"}
		}
		modelMap[id] = entry
	}

	// Also ensure web_search / explore / plan models have entries if different.
	for _, mid := range []string{
		nonEmpty(p.WebSearchModel, p.DefaultModel),
		explore,
		plan,
	} {
		if _, ok := modelMap[mid]; !ok {
			modelMap[mid] = map[string]interface{}{
				"model":                     mid,
				"api_key":                   p.APIKey,
				"supports_reasoning_effort": true,
				"reasoning_effort":          effort,
				"reasoning_efforts":         []interface{}{"low", "medium", "high"},
			}
		}
	}

	data["model"] = modelMap
}

// Match compares key config fields against a profile.
func Match(configPath string, p profiles.Profile) (bool, error) {
	data, err := Load(configPath)
	if err != nil {
		return false, err
	}
	apiURL := nestedString(data, "endpoints", "api_url")
	defModel := nestedString(data, "models", "default")
	apiKey := modelAPIKey(data, p.DefaultModel)
	if apiKey == "" {
		// Fallback: any model section key.
		apiKey = firstModelAPIKey(data)
	}

	base := strings.TrimRight(p.BaseURL, "/")
	got := strings.TrimRight(apiURL, "/")
	if base != got {
		return false, nil
	}
	if defModel != p.DefaultModel {
		return false, nil
	}
	if apiKey != p.APIKey {
		return false, nil
	}
	return true, nil
}

// ApplyOfficial removes mid-station related sections while preserving others.
// Must NOT wipe the entire file.
func ApplyOfficial(configPath string) error {
	data, err := Load(configPath)
	if err != nil {
		return err
	}
	delete(data, "endpoints")
	delete(data, "models")
	delete(data, "model")

	// Remove subagents.models but keep other subagents fields if any.
	if sub, ok := data["subagents"].(map[string]interface{}); ok {
		delete(sub, "models")
		if len(sub) == 0 {
			delete(data, "subagents")
		} else {
			data["subagents"] = sub
		}
	}

	return atomicWriteTOML(configPath, data)
}

// ImportFromConfig extracts a Profile skeleton from the current config.toml.
func ImportFromConfig(configPath, name string) (profiles.Profile, error) {
	data, err := Load(configPath)
	if err != nil {
		return profiles.Profile{}, err
	}
	baseURL := nestedString(data, "endpoints", "api_url")
	defModel := nestedString(data, "models", "default")
	webSearch := nestedString(data, "models", "web_search")
	explore := nestedString(data, "subagents", "models", "explore")
	plan := nestedString(data, "subagents", "models", "plan")
	apiKey := modelAPIKey(data, defModel)
	if apiKey == "" {
		apiKey = firstModelAPIKey(data)
	}
	effort := ""
	if m := modelSection(data, defModel); m != nil {
		effort = asString(m["reasoning_effort"])
	}

	p := profiles.Profile{
		Name:                   name,
		BaseURL:                baseURL,
		APIKey:                 apiKey,
		DefaultModel:           defModel,
		WebSearchModel:         webSearch,
		DefaultReasoningEffort: effort,
		SubagentsModels: profiles.SubagentsModels{
			Explore: explore,
			Plan:    plan,
		},
	}
	profiles.Normalize(&p)
	if err := profiles.Validate(p); err != nil {
		return profiles.Profile{}, fmt.Errorf("当前配置缺少必要字段，无法导入: %w", err)
	}
	return p, nil
}

// atomicWriteTOML marshals data and atomically replaces path.
func atomicWriteTOML(path string, data interface{}) error {
	raw, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化 TOML 失败: %w", err)
	}
	if err := atomicfile.Write(path, raw); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	// Read-back verification.
	if _, err := Load(path); err != nil {
		return fmt.Errorf("写入后校验失败: %w", err)
	}
	return nil
}

// AtomicWriteFile writes arbitrary bytes to path with 0600 and rename.
func AtomicWriteFile(path string, content []byte) error {
	return atomicfile.Write(path, content)
}

func nonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nestedString(data map[string]interface{}, keys ...string) string {
	var cur interface{} = data
	for _, k := range keys {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur, ok = m[k]
		if !ok {
			return ""
		}
	}
	return asString(cur)
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func modelSection(data map[string]interface{}, name string) map[string]interface{} {
	m, ok := data["model"].(map[string]interface{})
	if !ok {
		return nil
	}
	sec, ok := m[name].(map[string]interface{})
	if !ok {
		return nil
	}
	return sec
}

func modelAPIKey(data map[string]interface{}, name string) string {
	sec := modelSection(data, name)
	if sec == nil {
		return ""
	}
	return asString(sec["api_key"])
}

func firstModelAPIKey(data map[string]interface{}) string {
	m, ok := data["model"].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, v := range m {
		if sec, ok := v.(map[string]interface{}); ok {
			if k := asString(sec["api_key"]); k != "" {
				return k
			}
		}
	}
	return ""
}

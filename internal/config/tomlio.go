package config

import (
	"fmt"
	"os"
	"sort"
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

// Apply writes Profile fields into config.toml while preserving source layout
// and unknown fields, including unknown fields inside managed sections.
func Apply(configPath string, p profiles.Profile) error {
	raw, err := readConfigBytes(configPath)
	if err != nil {
		return err
	}
	if _, err := Load(configPath); err != nil {
		return err
	}
	rewritten, err := rewriteManagedTOML(raw, desiredManagedValues(p))
	if err != nil {
		return fmt.Errorf("更新配置失败: %w", err)
	}
	return atomicWriteAndVerify(configPath, rewritten)
}

// Match compares every field managed by Apply against a profile. Unknown TOML
// fields are deliberately ignored so future Grok settings remain compatible.
func Match(configPath string, p profiles.Profile) (bool, error) {
	data, err := Load(configPath)
	if err != nil {
		return false, err
	}
	if strings.TrimRight(nestedString(data, "endpoints", "api_url"), "/") != strings.TrimRight(p.BaseURL, "/") {
		return false, nil
	}
	if nestedString(data, "models", "default") != p.DefaultModel ||
		nestedString(data, "models", "web_search") != nonEmpty(p.WebSearchModel, p.DefaultModel) ||
		nestedString(data, "subagents", "models", "explore") != nonEmpty(p.SubagentsModels.Explore, p.DefaultModel) ||
		nestedString(data, "subagents", "models", "plan") != nonEmpty(p.SubagentsModels.Plan, p.DefaultModel) {
		return false, nil
	}

	expected := desiredModelSections(p)
	actual, ok := data["model"].(map[string]interface{})
	if !ok {
		return false, nil
	}
	actualManaged := make(map[string]map[string]interface{})
	for id, rawSection := range actual {
		section, ok := rawSection.(map[string]interface{})
		if !ok {
			continue
		}
		for key := range managedModelKeys {
			if _, exists := section[key]; exists {
				actualManaged[id] = section
				break
			}
		}
	}
	if len(actualManaged) != len(expected) {
		return false, nil
	}
	for id, want := range expected {
		got, exists := actualManaged[id]
		if !exists || !managedModelEqual(got, want) {
			return false, nil
		}
	}
	return true, nil
}

// IsOfficial reports whether none of the relay fields managed by grok-switch
// are present. Unknown settings do not make an otherwise official config fail.
func IsOfficial(configPath string) (bool, error) {
	data, err := Load(configPath)
	if err != nil {
		return false, err
	}
	return isOfficialData(data), nil
}

func isOfficialData(data map[string]interface{}) bool {
	for _, path := range [][]string{
		{"endpoints", "api_url"},
		{"models", "default"},
		{"models", "web_search"},
		{"subagents", "models", "explore"},
		{"subagents", "models", "plan"},
	} {
		if nestedExists(data, path...) {
			return false
		}
	}
	if modelMap, ok := data["model"].(map[string]interface{}); ok {
		for _, rawSection := range modelMap {
			section, ok := rawSection.(map[string]interface{})
			if !ok {
				continue
			}
			for key := range managedModelKeys {
				if _, exists := section[key]; exists {
					return false
				}
			}
		}
	}
	return true
}

// ApplyOfficial removes mid-station related sections while preserving others.
// Must NOT wipe the entire file.
func ApplyOfficial(configPath string) error {
	raw, err := readConfigBytes(configPath)
	if err != nil {
		return err
	}
	if _, err := Load(configPath); err != nil {
		return err
	}
	rewritten, err := removeManagedTOML(raw)
	if err != nil {
		return fmt.Errorf("更新官方配置失败: %w", err)
	}
	var rewrittenData map[string]interface{}
	if len(strings.TrimSpace(string(rewritten))) > 0 {
		if err := toml.Unmarshal(rewritten, &rewrittenData); err != nil {
			return fmt.Errorf("更新官方配置失败: %w", err)
		}
	}
	if rewrittenData == nil {
		rewrittenData = map[string]interface{}{}
	}
	if !isOfficialData(rewrittenData) {
		return fmt.Errorf("更新官方配置失败: 当前 TOML 使用了不支持的内联受管表格式")
	}
	return atomicWriteAndVerify(configPath, rewritten)
}

// ImportFromConfig extracts a Profile skeleton from the current config.toml.
func ImportFromConfig(configPath, name string) (profiles.Profile, error) {
	data, err := Load(configPath)
	if err != nil {
		return profiles.Profile{}, err
	}
	defModel := nestedString(data, "models", "default")
	model := modelSection(data, defModel)
	baseURL := ""
	backend := ""
	if model != nil {
		baseURL = asString(model["base_url"])
		backend = asString(model["api_backend"])
	}
	// Grok Build 0.2.106 resolves routing per model. Fall back to the legacy
	// global endpoint so profiles created by older grok-switch releases remain
	// importable.
	if baseURL == "" {
		baseURL = nestedString(data, "endpoints", "api_url")
	}
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
		UpstreamFormat:         upstreamFormatForBackend(backend),
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

func readConfigBytes(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	return raw, nil
}

func atomicWriteAndVerify(path string, raw []byte) error {
	if err := atomicfile.Write(path, raw); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if _, err := Load(path); err != nil {
		return fmt.Errorf("写入后校验失败: %w", err)
	}
	return nil
}

func desiredManagedValues(p profiles.Profile) []managedValue {
	explore := nonEmpty(p.SubagentsModels.Explore, p.DefaultModel)
	plan := nonEmpty(p.SubagentsModels.Plan, p.DefaultModel)
	values := []managedValue{
		{path: []string{"endpoints", "api_url"}, value: p.BaseURL},
		{path: []string{"models", "default"}, value: p.DefaultModel},
		{path: []string{"models", "web_search"}, value: nonEmpty(p.WebSearchModel, p.DefaultModel)},
		{path: []string{"subagents", "models", "explore"}, value: explore},
		{path: []string{"subagents", "models", "plan"}, value: plan},
	}
	sections := desiredModelSections(p)
	for _, id := range sortedModelIDs(sections) {
		section := sections[id]
		for _, key := range []string{"model", "base_url", "api_backend", "api_key", "supports_reasoning_effort", "reasoning_effort", "reasoning_efforts"} {
			values = append(values, managedValue{path: []string{"model", id, key}, value: section[key]})
		}
	}
	return values
}

func desiredModelSections(p profiles.Profile) map[string]map[string]interface{} {
	effort := nonEmpty(p.DefaultReasoningEffort, "high")
	baseURL := grokModelBaseURL(p.BaseURL)
	backend := apiBackendForUpstreamFormat(p.UpstreamFormat)
	sections := map[string]map[string]interface{}{
		p.DefaultModel: {
			"model":                     p.DefaultModel,
			"base_url":                  baseURL,
			"api_backend":               backend,
			"api_key":                   p.APIKey,
			"supports_reasoning_effort": true,
			"reasoning_effort":          effort,
			"reasoning_efforts":         []string{"low", "medium", "high"},
		},
	}
	for _, m := range p.Models {
		id := nonEmpty(m.ID, m.Model)
		if id == "" {
			continue
		}
		key := nonEmpty(m.APIKey, p.APIKey)
		reasoningEffort := nonEmpty(m.ReasoningEffort, effort)
		reasoningEfforts := append([]string(nil), m.ReasoningEfforts...)
		if len(reasoningEfforts) == 0 {
			reasoningEfforts = []string{"low", "medium", "high"}
		}
		sections[id] = map[string]interface{}{
			"model":                     nonEmpty(m.Model, id),
			"base_url":                  baseURL,
			"api_backend":               backend,
			"api_key":                   key,
			"supports_reasoning_effort": m.SupportsReasoningEffort,
			"reasoning_effort":          reasoningEffort,
			"reasoning_efforts":         reasoningEfforts,
		}
	}
	for _, id := range []string{nonEmpty(p.WebSearchModel, p.DefaultModel), nonEmpty(p.SubagentsModels.Explore, p.DefaultModel), nonEmpty(p.SubagentsModels.Plan, p.DefaultModel)} {
		if _, ok := sections[id]; !ok {
			sections[id] = map[string]interface{}{
				"model":                     id,
				"base_url":                  baseURL,
				"api_backend":               backend,
				"api_key":                   p.APIKey,
				"supports_reasoning_effort": true,
				"reasoning_effort":          effort,
				"reasoning_efforts":         []string{"low", "medium", "high"},
			}
		}
	}
	return sections
}

func sortedModelIDs(sections map[string]map[string]interface{}) []string {
	ids := make([]string, 0, len(sections))
	for id := range sections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func managedModelEqual(got, want map[string]interface{}) bool {
	for key := range managedModelKeys {
		if _, exists := got[key]; !exists {
			return false
		}
	}
	for _, key := range []string{"model", "base_url", "api_backend", "api_key", "reasoning_effort"} {
		if asString(got[key]) != asString(want[key]) {
			return false
		}
	}
	if asBool(got["supports_reasoning_effort"]) != asBool(want["supports_reasoning_effort"]) {
		return false
	}
	return equalStringSlice(asStringSlice(got["reasoning_efforts"]), asStringSlice(want["reasoning_efforts"]))
}

// grokModelBaseURL returns the OpenAI-compatible API prefix consumed by
// Grok Build's per-model base_url. Profiles historically accepted either a
// host root or a full /v1 prefix, while Grok Build expects the latter.
func grokModelBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.HasSuffix(baseURL, "/v1") || strings.Contains(baseURL, "/v1/") {
		return baseURL
	}
	return baseURL + "/v1"
}

func apiBackendForUpstreamFormat(format string) string {
	switch strings.TrimSpace(format) {
	case "", "openai_chat", "chat_completions":
		return "chat_completions"
	default:
		return strings.TrimSpace(format)
	}
}

func upstreamFormatForBackend(backend string) string {
	switch strings.TrimSpace(backend) {
	case "", "chat_completions":
		return "openai_chat"
	default:
		return strings.TrimSpace(backend)
	}
}

func nestedExists(data map[string]interface{}, keys ...string) bool {
	var current interface{} = data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		current, ok = m[key]
		if !ok {
			return false
		}
	}
	return true
}

func asBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func asStringSlice(v interface{}) []string {
	switch values := v.(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, len(values))
		for i, value := range values {
			out[i] = asString(value)
		}
		return out
	default:
		return nil
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	case nil:
		return ""
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

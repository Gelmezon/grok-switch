package profiles

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	catalog "github.com/Gelmezon/grok-switch/internal/models"
)

// Profile represents a Grok Build mid-station profile.
type Profile struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	UpstreamFormat         string          `json:"upstream_format"`
	BaseURL                string          `json:"base_url"`
	APIKey                 string          `json:"api_key"`
	AvailableModels        []string        `json:"available_models"`
	DefaultModel           string          `json:"default_model"`
	DefaultReasoningEffort string          `json:"default_reasoning_effort"`
	WebSearchModel         string          `json:"web_search_model"`
	SubagentsModels        SubagentsModels `json:"subagents_models"`
	Models                 []ModelDef      `json:"models"`
	DisableCodebaseUpload  *bool           `json:"disable_codebase_upload,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
	IsActive               bool            `json:"is_active"`
}

// CodebaseUploadDisabled reports the effective privacy setting. A missing
// value represents the secure default so profiles created by older releases
// are protected automatically after upgrading.
func (p Profile) CodebaseUploadDisabled() bool {
	return p.DisableCodebaseUpload == nil || *p.DisableCodebaseUpload
}

// SetCodebaseUploadDisabled records an explicit privacy choice.
func (p *Profile) SetCodebaseUploadDisabled(disabled bool) {
	p.DisableCodebaseUpload = new(bool)
	*p.DisableCodebaseUpload = disabled
}

// SubagentsModels holds model IDs for subagent roles.
type SubagentsModels struct {
	Explore string `json:"explore"`
	Plan    string `json:"plan"`
}

// ModelDef is an optional per-model definition stored with a profile.
type ModelDef struct {
	ID                      string   `json:"id"`
	Model                   string   `json:"model"`
	APIKey                  string   `json:"api_key,omitempty"`
	SupportsReasoningEffort bool     `json:"supports_reasoning_effort"`
	ReasoningEffort         string   `json:"reasoning_effort,omitempty"`
	ReasoningEfforts        []string `json:"reasoning_efforts,omitempty"`
}

// Normalize fills defaults and trims BaseURL trailing slashes.
// Called on Create/Update.
func Normalize(p *Profile) {
	if p.UpstreamFormat == "" {
		p.UpstreamFormat = "openai_chat"
	}
	if p.DefaultReasoningEffort == "" {
		p.DefaultReasoningEffort = "high"
	}
	if p.WebSearchModel == "" {
		p.WebSearchModel = p.DefaultModel
	}
	if p.SubagentsModels.Explore == "" {
		p.SubagentsModels.Explore = p.DefaultModel
	}
	if p.SubagentsModels.Plan == "" {
		p.SubagentsModels.Plan = p.DefaultModel
	}
	// Remove trailing slashes; do NOT auto-append /v1.
	p.BaseURL = strings.TrimRight(p.BaseURL, "/")
	p.Name = strings.TrimSpace(p.Name)
	if p.AvailableModels == nil {
		p.AvailableModels = []string{}
	}
	if p.Models == nil {
		p.Models = []ModelDef{}
	}
}

// Validate checks required fields.
func Validate(p Profile) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("Name 不能为空")
	}
	if p.BaseURL == "" {
		return fmt.Errorf("BaseURL 不能为空")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("BaseURL 必须是合法的 http:// 或 https:// URL")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return fmt.Errorf("APIKey 不能为空")
	}
	if strings.TrimSpace(p.DefaultModel) == "" {
		return fmt.Errorf("DefaultModel 不能为空")
	}
	return nil
}

// ApplyDiscoveredModels replaces the profile's model inventory with IDs
// returned by the relay's /models endpoint. Existing per-model overrides are
// preserved for IDs that remain available.
func ApplyDiscoveredModels(p *Profile, discovered []string) error {
	seen := make(map[string]bool, len(discovered))
	ids := make([]string, 0, len(discovered))
	for _, id := range discovered {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return fmt.Errorf("中间站 /models 未返回可用模型")
	}
	sort.Strings(ids)

	defaultModel := strings.TrimSpace(p.DefaultModel)
	if !seen[defaultModel] {
		defaultModel = ""
		for _, preferred := range []string{"grok-4.5-latest", "grok-4.5", catalog.DefaultModel} {
			if seen[preferred] {
				defaultModel = preferred
				break
			}
		}
		if defaultModel == "" {
			defaultModel = ids[0]
		}
	}

	existing := make(map[string]ModelDef, len(p.Models))
	for _, model := range p.Models {
		id := strings.TrimSpace(nonEmptyModelID(model))
		if id != "" {
			existing[id] = model
		}
	}
	models := make([]ModelDef, 0, len(ids))
	for _, id := range ids {
		model, ok := existing[id]
		if !ok {
			model = ModelDef{
				ID:                      id,
				Model:                   id,
				SupportsReasoningEffort: true,
				ReasoningEffort:         "high",
				ReasoningEfforts:        []string{"low", "medium", "high"},
			}
		}
		model.ID = id
		if strings.TrimSpace(model.Model) == "" {
			model.Model = id
		}
		models = append(models, model)
	}

	p.AvailableModels = ids
	p.Models = models
	p.DefaultModel = defaultModel
	if !seen[p.WebSearchModel] {
		p.WebSearchModel = defaultModel
	}
	if !seen[p.SubagentsModels.Explore] {
		p.SubagentsModels.Explore = defaultModel
	}
	if !seen[p.SubagentsModels.Plan] {
		p.SubagentsModels.Plan = defaultModel
	}
	Normalize(p)
	return nil
}

func nonEmptyModelID(model ModelDef) string {
	if strings.TrimSpace(model.ID) != "" {
		return model.ID
	}
	return model.Model
}

// Public returns a copy safe for JSON list output (no API key).
func (p Profile) Public() map[string]interface{} {
	return map[string]interface{}{
		"id":                       p.ID,
		"name":                     p.Name,
		"upstream_format":          p.UpstreamFormat,
		"base_url":                 p.BaseURL,
		"available_models":         p.AvailableModels,
		"default_model":            p.DefaultModel,
		"default_reasoning_effort": p.DefaultReasoningEffort,
		"web_search_model":         p.WebSearchModel,
		"subagents_models":         p.SubagentsModels,
		"disable_codebase_upload":  p.CodebaseUploadDisabled(),
		"created_at":               p.CreatedAt,
		"updated_at":               p.UpdatedAt,
		"is_active":                p.IsActive,
	}
}

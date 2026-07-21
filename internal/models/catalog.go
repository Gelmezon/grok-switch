package models

// Catalog is the built-in list of common Grok models for selection UIs.
// Users may still pick "custom" to type a model id.
var Catalog = []string{
	"grok-4",
	"grok-4-0709",
	"grok-4-fast",
	"grok-4-fast-non-reasoning",
	"grok-3",
	"grok-3-mini",
	"grok-3-fast",
	"grok-2",
	"grok-2-vision-1212",
}

// ReasoningEfforts is the selectable reasoning effort list.
var ReasoningEfforts = []string{"low", "medium", "high"}

// DefaultModel is the default selection for new providers.
const DefaultModel = "grok-4"

// DefaultReasoningEffort is the default effort.
const DefaultReasoningEffort = "high"

// OfficialName is the display name for the built-in official provider.
const OfficialName = "官方"

// OfficialID is a synthetic list id (not stored in profiles.json).
const OfficialID = "__official__"

// IndexOf returns the catalog index of model, or -1.
func IndexOf(model string) int {
	for i, m := range Catalog {
		if m == model {
			return i
		}
	}
	return -1
}

// IndexOfEffort returns effort index, default high.
func IndexOfEffort(e string) int {
	for i, v := range ReasoningEfforts {
		if v == e {
			return i
		}
	}
	return 2 // high
}

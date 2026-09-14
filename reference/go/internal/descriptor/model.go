package descriptor

import "fmt"

// Model describes one text model. Omitted properties are unknown, not defaults.
type Model struct {
	ID               string    `json:"id"`
	API              *ModelAPI `json:"api,omitempty"`
	Name             *string   `json:"name,omitempty"`
	Reasoning        *bool     `json:"reasoning,omitempty"`
	ReasoningEfforts []string  `json:"reasoning_efforts,omitempty"`
	ContextWindow    *int64    `json:"context_window,omitempty"`
	MaxOutputTokens  *int64    `json:"max_output_tokens,omitempty"`
}

// ModelAPI replaces the provider defaults for this model.
type ModelAPI struct {
	Profiles     []string `json:"profiles"`
	Capabilities []string `json:"capabilities"`
}

func (d Descriptor) APIFor(m Model) ModelAPI {
	if m.API != nil {
		return *m.API
	}
	return ModelAPI{Profiles: d.API.Profiles, Capabilities: d.API.Capabilities}
}

const maxTokens = 2147483647

func ValidateModels(models []Model) error {
	if len(models) == 0 || len(models) > 128 {
		return fmt.Errorf("api.models must contain 1..128 models; configure a smaller model list if needed")
	}
	seen := make(map[string]bool, len(models))
	for _, m := range models {
		if !Text(m.ID, 256) || seen[m.ID] {
			return fmt.Errorf("model IDs must be unique, printable text of 1..256 bytes")
		}
		seen[m.ID] = true
		if m.API != nil && (!tokens(m.API.Profiles, false) || !tokens(m.API.Capabilities, true)) {
			return fmt.Errorf("model.api requires profiles and capabilities")
		}
		if m.Name != nil && !Text(*m.Name, 128) {
			return fmt.Errorf("invalid model name")
		}
		if m.ReasoningEfforts != nil && (!tokens(m.ReasoningEfforts, false) || m.Reasoning == nil || !*m.Reasoning) {
			return fmt.Errorf("reasoning_efforts requires reasoning: true and unique effort identifiers")
		}
		for _, limit := range []*int64{m.ContextWindow, m.MaxOutputTokens} {
			if limit != nil && (*limit < 1 || *limit > maxTokens) {
				return fmt.Errorf("model token limits must be between 1 and %d when supplied", maxTokens)
			}
		}
		if m.ContextWindow != nil && m.MaxOutputTokens != nil && *m.MaxOutputTokens > *m.ContextWindow {
			return fmt.Errorf("max_output_tokens exceeds context_window")
		}
	}
	return nil
}

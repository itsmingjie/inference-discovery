package descriptor

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func Parse(b []byte) (Descriptor, error) {
	var d Descriptor
	if len(b) > MaxBytes {
		return d, fmt.Errorf("descriptor exceeds %d bytes", MaxBytes)
	}
	if err := CheckJSON(b); err != nil {
		return d, fmt.Errorf("invalid descriptor: %w", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil || root == nil {
		return d, fmt.Errorf("descriptor must be an object")
	}
	var api, auth map[string]json.RawMessage
	if json.Unmarshal(root["api"], &api) != nil || api == nil || json.Unmarshal(root["auth"], &auth) != nil || auth == nil {
		return d, fmt.Errorf("api and auth must be objects")
	}
	// Decode only exact field names: encoding/json struct matching is otherwise
	// case insensitive, allowing unknown extensions to overwrite required fields.
	decode := func(m map[string]json.RawMessage, k string, v any) error {
		raw, ok := m[k]
		if !ok || string(raw) == "null" {
			return fmt.Errorf("missing or null %s", k)
		}
		return json.Unmarshal(raw, v)
	}
	// Check the numeric value without accepting strings or rounding to version 1.
	version, err := integer(root["version"], 1, 1)
	if err != nil {
		return d, fmt.Errorf("unsupported descriptor version")
	}
	d.Version = int(version)
	for _, field := range []struct {
		m map[string]json.RawMessage
		k string
		v any
	}{{root, "name", &d.Name}, {api, "base_url", &d.API.BaseURL}, {api, "profiles", &d.API.Profiles}, {api, "capabilities", &d.API.Capabilities}, {auth, "methods", &d.Auth.Methods}} {
		if err := decode(field.m, field.k, field.v); err != nil {
			return d, fmt.Errorf("invalid %s: %w", field.k, err)
		}
	}
	var models []json.RawMessage
	if err := decode(api, "models", &models); err != nil {
		return d, fmt.Errorf("invalid models: %w", err)
	}
	d.API.Models = make([]Model, 0, len(models))
	for _, raw := range models {
		model, err := ParseModel(raw)
		if err != nil {
			return d, fmt.Errorf("invalid model: %w", err)
		}
		d.API.Models = append(d.API.Models, model)
	}
	if _, ok := api["default_model"]; ok {
		if err := decode(api, "default_model", &d.API.DefaultModel); err != nil || !Text(d.API.DefaultModel, 256) {
			return d, fmt.Errorf("invalid api.default_model")
		}
	}
	return d, d.Validate()
}

// integer accepts JSON integer values in decimal or exponent notation without
// rounding. The range check also bounds exponent expansion before using big.Rat.
func integer(raw json.RawMessage, min, max int64) (int64, error) {
	text := strings.TrimSpace(string(raw))
	approx, err := strconv.ParseFloat(text, 64)
	if err != nil || approx < float64(min) || approx > float64(max) {
		return 0, fmt.Errorf("expected an integer between %d and %d", min, max)
	}
	exact, ok := new(big.Rat).SetString(text)
	if !ok || !exact.IsInt() || !exact.Num().IsInt64() {
		return 0, fmt.Errorf("expected an integer between %d and %d", min, max)
	}
	n := exact.Num().Int64()
	if n < min || n > max {
		return 0, fmt.Errorf("integer outside allowed range")
	}
	return n, nil
}

// ParseModel applies wire-only rules: exact field names, ignored extensions,
// no null properties, and numeric values independent of JSON number notation.
func ParseModel(b json.RawMessage) (Model, error) {
	var m Model
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil || fields == nil {
		return m, fmt.Errorf("model must be an object")
	}
	if raw, ok := fields["api"]; ok {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return m, fmt.Errorf("model.api must be an object")
		}
		m.API = &ModelAPI{}
		if json.Unmarshal(fields["profiles"], &m.API.Profiles) != nil || json.Unmarshal(fields["capabilities"], &m.API.Capabilities) != nil {
			return m, fmt.Errorf("invalid model.api")
		}
	}
	for _, field := range []struct {
		name  string
		value any
	}{
		{"id", &m.ID}, {"name", &m.Name}, {"reasoning", &m.Reasoning},
		{"reasoning_efforts", &m.ReasoningEfforts},
	} {
		if raw, ok := fields[field.name]; ok {
			if string(raw) == "null" {
				return m, fmt.Errorf("model.%s must not be null", field.name)
			}
			if err := json.Unmarshal(raw, field.value); err != nil {
				return m, fmt.Errorf("model.%s: %w", field.name, err)
			}
		}
	}
	for _, field := range []struct {
		name  string
		value **int64
	}{
		{"context_window", &m.ContextWindow}, {"max_output_tokens", &m.MaxOutputTokens},
	} {
		if raw, ok := fields[field.name]; ok {
			n, err := integer(raw, 1, maxTokens)
			if err != nil {
				return m, fmt.Errorf("model.%s: %w", field.name, err)
			}
			*field.value = &n
		}
	}
	return m, nil
}

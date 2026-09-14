package inference

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

// Catalog reads only model properties relevant to discovery. Vendor pricing,
// links, headers and credentials are never retained or followed.
func (c Client) Catalog(ctx context.Context) ([]descriptor.Model, error) {
	res, err := c.request(ctx, http.MethodGet, "models", nil)
	if err != nil {
		return nil, err
	}
	b, err := transport.Read(res, MaxResponse, "application/json")
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	if response.Data == nil || len(response.Data) > 4096 {
		return nil, fmt.Errorf("models response must contain a data array with at most 4096 entries")
	}
	models := make([]descriptor.Model, 0, len(response.Data))
	for _, raw := range response.Data {
		fields := map[string]json.RawMessage{}
		for _, key := range []string{"id", "name", "reasoning", "reasoning_efforts", "context_window", "max_output_tokens", "api"} {
			if value := raw[key]; len(value) > 0 && string(value) != "null" {
				fields[key] = value
			}
		}
		// OpenRouter-compatible catalogs use these names for the same limits.
		if fields["context_window"] == nil && string(raw["context_length"]) != "null" {
			fields["context_window"] = raw["context_length"]
		}
		var top map[string]json.RawMessage
		if json.Unmarshal(raw["top_provider"], &top) == nil && fields["max_output_tokens"] == nil && string(top["max_completion_tokens"]) != "null" {
			fields["max_output_tokens"] = top["max_completion_tokens"]
		}
		var parameters []string
		if fields["reasoning"] == nil && json.Unmarshal(raw["supported_parameters"], &parameters) == nil && (slices.Contains(parameters, "reasoning") || slices.Contains(parameters, "reasoning_effort")) {
			fields["reasoning"] = json.RawMessage("true")
		}
		// Remove absent aliases before applying the descriptor's strict parser.
		for key, value := range fields {
			if len(value) == 0 {
				delete(fields, key)
			}
		}
		b, _ := json.Marshal(fields)
		m, err := descriptor.ParseModel(b)
		if err != nil {
			return nil, fmt.Errorf("model metadata: %w", err)
		}
		if err := descriptor.ValidateModels([]descriptor.Model{m}); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	slices.SortFunc(models, func(a, b descriptor.Model) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return slices.CompactFunc(models, func(a, b descriptor.Model) bool { return a.ID == b.ID }), nil
}

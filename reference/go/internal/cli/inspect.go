package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
)

func inspect(ctx context.Context, selected provider, stream bool, out io.Writer) error {
	compatibility := selected.Descriptor.Compatible(stream)
	if compatibility == nil {
		_, compatibility = authentication.None{}.Resolve(ctx, *selected.Descriptor)
	}
	report := map[string]any{"descriptor_url": selected.URL, "descriptor": json.RawMessage(selected.Raw), "compatible": compatibility == nil}
	if compatibility != nil {
		report["error"] = compatibility.Error()
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	return compatibility
}

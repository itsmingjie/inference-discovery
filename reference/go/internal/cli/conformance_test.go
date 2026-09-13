package cli

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

// Exercise the connection contract shared with future non-Go implementations.
func TestConformance(t *testing.T) {
	data, err := os.ReadFile("../../../../conformance/fixtures/descriptors.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name                string          `json:"name"`
		Descriptor          json.RawMessage `json:"descriptor"`
		Raw                 string          `json:"raw"`
		Valid               bool            `json:"valid"`
		StreamingConnection bool            `json:"streaming_connection"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			payload := tc.Descriptor
			if tc.Raw != "" {
				payload = []byte(tc.Raw)
			}
			d, err := descriptor.Parse(payload)
			if (err == nil) != tc.Valid {
				t.Fatalf("valid=%v: %v", tc.Valid, err)
			}
			if err != nil {
				return
			}
			_, err = authentication.None{}.Resolve(context.Background(), d)
			compatible := err == nil && d.Compatible(true) == nil
			if compatible != tc.StreamingConnection {
				t.Fatalf("streaming connection: got %v, want %v", compatible, tc.StreamingConnection)
			}
		})
	}
}

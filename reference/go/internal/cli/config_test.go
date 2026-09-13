package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigModelBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	for _, tc := range []struct{ model, wantError string }{
		{`{"id":"chat","context_windwo":131072}`, "unknown field"},
		{`{"id":"chat","context_window":0}`, "token limits"},
		{`{"id":"chat","name":""}`, "model name"},
		{`{"id":"chat","name":"Office Chat","reasoning":false,"context_window":131072}`, ""},
	} {
		t.Run(tc.model, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(`{"name":"From file","models":[`+tc.model+`]}`), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := parseAdvertiseConfig([]string{"--config", path, "--name", "From flag"}, io.Discard)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %q, got %v", tc.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Name != "From flag" || cfg.Models[0].ContextWindow == nil || *cfg.Models[0].ContextWindow != 131072 || cfg.Models[0].Reasoning == nil || *cfg.Models[0].Reasoning {
				t.Fatalf("configuration values or flag precedence lost: %+v", cfg)
			}
		})
	}
}

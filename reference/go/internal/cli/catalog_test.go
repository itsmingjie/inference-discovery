package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func TestAdvertiserCatalogRefresh(t *testing.T) {
	response := `{"data":[{"id":"chat","pricing":{"prompt":"1"}},{"id":"reasoner"},{"id":"embedding"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected model request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	ctx := context.Background()
	cfg := advertiseConfig{Name: "Office", Endpoint: server.URL + "/v1"}
	session, err := (authentication.None{}).Resolve(ctx, descriptor.Descriptor{
		API: descriptor.API{BaseURL: cfg.Endpoint}, Auth: descriptor.Auth{Methods: []string{"none"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := transport.Client(time.Second)
	defer client.CloseIdleConnections()
	api := inference.Client{HTTP: client, Base: cfg.Endpoint, Session: session}
	b, err := providerDescriptor(ctx, cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	d, err := descriptor.Parse(b)
	if err != nil || len(d.API.Models) != 3 || strings.Contains(string(b), "pricing") {
		t.Fatalf("unconfigured catalog: %s (%v)", b, err)
	}
	reasoning := true
	contextWindow, maxOutput := int64(131072), int64(16384)
	cfg.Models = []descriptor.Model{{ID: "chat"}, {ID: "reasoner", Reasoning: &reasoning,
		ReasoningEfforts: []string{"low", "high"}, ContextWindow: &contextWindow, MaxOutputTokens: &maxOutput}}
	cfg.Model = "reasoner"
	b, err = providerDescriptor(ctx, cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	d, err = descriptor.Parse(b)
	if err != nil || len(d.API.Models) != 2 || d.API.DefaultModel != "reasoner" || (d.API.Models[1].ContextWindow == nil || *d.API.Models[1].ContextWindow != 131072) {
		t.Fatalf("configured catalog: %s (%v)", b, err)
	}
	response = `{"data":[{"id":"chat"}]}`
	if _, err := providerDescriptor(ctx, cfg, api); err == nil {
		t.Fatal("unavailable explicit default must fail the health check")
	}
	cfg.Model = ""
	b, err = providerDescriptor(ctx, cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	d, err = descriptor.Parse(b)
	if err != nil || len(d.API.Models) != 1 || d.API.DefaultModel != "chat" {
		t.Fatalf("stale model retained: %s (%v)", b, err)
	}
	response = `{"data":[{"id":"embedding"}]}`
	if _, err := providerDescriptor(ctx, cfg, api); err == nil {
		t.Fatal("empty configured catalog must fail the health check")
	}
}

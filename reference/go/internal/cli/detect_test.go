package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func TestChatSelectsCompatibleModelInMixedCatalog(t *testing.T) {
	d := descriptor.Descriptor{Version: 1, Name: "Mixed", Auth: descriptor.Auth{Methods: []string{"none"}}, API: descriptor.API{
		BaseURL: "http://192.0.2.1/v1", Profiles: []string{}, Capabilities: []string{"streaming"}, DefaultModel: "responses",
		Models: []descriptor.Model{
			{ID: "responses", API: &descriptor.ModelAPI{Profiles: []string{descriptor.ResponsesProfile}, Capabilities: []string{"streaming"}}},
			{ID: "chat", API: &descriptor.ModelAPI{Profiles: []string{descriptor.Profile}, Capabilities: []string{"streaming"}}},
		},
	}}
	var output bytes.Buffer
	if err := chat(context.Background(), provider{Descriptor: &d}, options{chatTimeout: time.Second}, bufio.NewScanner(strings.NewReader("/quit\n")), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Model: chat") {
		t.Fatalf("did not select the compatible default: %s", output.String())
	}
	if err := chat(context.Background(), provider{Descriptor: &d}, options{model: "responses"}, bufio.NewScanner(strings.NewReader("/quit\n")), &output); err == nil {
		t.Fatal("silently replaced an incompatible explicit selection")
	}
}

func TestAutomaticCatalogDetection(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	brokenHealthy := false
	listing := `{"data":[{"id":"text"},{"id":"tools"},{"id":"both"},{"id":"responses"},{"id":"broken"},{"id":"metadata","context_length":32768,"top_provider":{"max_completion_tokens":4096},"supported_parameters":["reasoning"],"pricing":{"input":"secret-price"},"api":{"profiles":["openai-chat-completions"],"capabilities":["streaming","function-tools"]}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent in probe")
		}
		if r.URL.Path == "/prefix/v1/models" {
			mu.Lock()
			body := listing
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, body)
			return
		}
		var request struct {
			Model  string            `json:"model"`
			Tools  []json.RawMessage `json:"tools"`
			Stream bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || !request.Stream {
			t.Error("invalid probe request")
			http.Error(w, "invalid", 400)
			return
		}
		mu.Lock()
		requests[request.Model]++
		healthy := brokenHealthy
		mu.Unlock()
		responses := r.URL.Path == "/prefix/v1/responses"
		if r.URL.Path != "/prefix/v1/chat/completions" && !responses {
			t.Error("unexpected route", r.URL.Path)
		}
		if (responses && request.Model != "responses" && request.Model != "both") || (!responses && request.Model == "responses") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if request.Model == "broken" && !healthy {
			fmt.Fprint(w, "data: not-json\n\n")
			return
		}
		tool := len(request.Tools) > 0 && request.Model != "text"
		if responses {
			if tool {
				fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"name\":\"inference_probe\",\"call_id\":\"call-1\",\"arguments\":\"{\\\"ok\\\":true}\"}}\n\n")
			} else {
				fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
			}
			fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		} else {
			if tool {
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"inference_probe\",\"arguments\":\"{\\\"ok\\\":true}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
			} else {
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	defer server.Close()
	ctx := context.Background()
	base := server.URL + "/prefix/v1"
	session, err := (authentication.None{}).Resolve(ctx, descriptor.Descriptor{API: descriptor.API{BaseURL: base}, Auth: descriptor.Auth{Methods: []string{"none"}}})
	if err != nil {
		t.Fatal(err)
	}
	client := transport.Client(time.Second)
	defer client.CloseIdleConnections()
	var diagnostics bytes.Buffer
	builder := catalogBuilder{cfg: advertiseConfig{Name: "Office", Endpoint: base}, api: inference.Client{HTTP: client, Base: base, Session: session}, timeout: time.Second, out: &diagnostics}
	body, err := builder.build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := descriptor.Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.API.Models) != 5 || len(d.API.Profiles) != 0 {
		t.Fatalf("incorrect common profile or catalog: %s", body)
	}
	for _, m := range d.API.Models {
		a := d.APIFor(m)
		if slices.Contains(a.Capabilities, "function-tools") != (m.ID == "tools" || m.ID == "both" || m.ID == "metadata") {
			t.Fatalf("incorrect tool capability for %s", m.ID)
		}
		if m.ID == "both" && len(a.Profiles) != 2 {
			t.Fatal("lost one supported API")
		}
		if m.ID == "metadata" && (m.ContextWindow == nil || *m.ContextWindow != 32768 || m.MaxOutputTokens == nil || *m.MaxOutputTokens != 4096 || m.Reasoning == nil || !*m.Reasoning) {
			t.Fatal("model metadata lost")
		}
	}
	if bytes.Contains(body, []byte("pricing")) || bytes.Contains(body, []byte("secret-price")) {
		t.Fatal("pricing copied")
	}
	if !bytes.Contains(diagnostics.Bytes(), []byte("broken skipped")) {
		t.Fatal("inconclusive detection was silent")
	}
	mu.Lock()
	initial := make(map[string]int)
	for id, count := range requests {
		initial[id] = count
	}
	mu.Unlock()
	if initial["both"] != 4 || initial["text"] != 3 || initial["metadata"] != 0 {
		t.Fatalf("wrong probe count: %v", initial)
	}
	if _, err := builder.build(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	for id, count := range initial {
		if requests[id] != count {
			t.Error("health poll repeated generation", id)
		}
	}
	brokenHealthy = true
	mu.Unlock()
	// A transient failure retries after its cooldown without reproving known models.
	builder.checked["broken"] = modelCheck{retry: time.Now().Add(-time.Second)}
	body, err = builder.build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err = descriptor.Parse(body)
	if err != nil || len(d.API.Models) != 6 {
		t.Fatalf("recovered model missing: %s (%v)", body, err)
	}
	mu.Lock()
	for id, count := range initial {
		if id != "broken" && requests[id] != count {
			t.Error("recovery reprobed a known model", id)
		}
	}
	listing = `{"data":[{"id":"text"}]}`
	mu.Unlock()
	body, err = builder.build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err = descriptor.Parse(body)
	if err != nil || len(d.API.Models) != 1 {
		t.Fatalf("disappearing model retained: %s (%v)", body, err)
	}
	if len(builder.checked) != 1 {
		t.Fatal("disappeared models retained in probe cache")
	}
	mu.Lock()
	listing = `{"data":[]}`
	mu.Unlock()
	if _, err := builder.build(ctx); err == nil || len(builder.checked) != 0 {
		t.Fatal("empty upstream catalog must withdraw and forget previous checks")
	}
	mu.Lock()
	listing = `{"data":[{"id":"text"}]}`
	mu.Unlock()
	// A manual assertion needs only /models, even when automatic checking failed.
	builder.cfg.Models = []descriptor.Model{{ID: "text", API: &descriptor.ModelAPI{Profiles: []string{descriptor.ResponsesProfile}, Capabilities: []string{"streaming"}}}}
	mu.Lock()
	before := requests["text"]
	mu.Unlock()
	if _, err := builder.build(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests["text"] != before {
		t.Fatal("explicit model metadata generated a probe")
	}
}

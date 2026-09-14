package inference

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
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func TestSSE(t *testing.T) {
	valid := ": ping\r\n\r\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\r\n\r\ndata: {\"choices\":\ndata: [{\"index\":0,\"delta\":{\"content\":\"Hello 🌍\"}}]}\n\ndata: {\"choices\":[],\"usage\":{}}\n\ndata: [DONE]\n\n"
	var got strings.Builder
	s, err := readStream(strings.NewReader(valid), func(s string) error { got.WriteString(s); return nil })
	if err != nil || s != "Hello 🌍" || got.String() != s {
		t.Fatal(s, err)
	}
	for _, s := range []string{"data: {}\n\n", "data: nope\n\n", "data: {\"error\":{}}\n\n", "data: {\"choices\":[{\"delta\":{\"content\":123}}]}\n\n", "data: [DONE]", strings.Repeat("x", 65537), "data: {\"choices\":[]}\n\n"} {
		if _, err = readStream(strings.NewReader(s), func(string) error { return nil }); err == nil {
			t.Fatalf("accepted %.100s", s)
		}
	}
}

func TestEndpointAndNoReplay(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials leaked")
		}
		switch r.URL.Path {
		case "/prefix/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"}]}`)
		case "/prefix/v1/chat/completions":
			calls++
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		default:
			t.Error("wrong path", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	d := descriptor.Descriptor{Version: 1, Name: "Test", API: descriptor.API{BaseURL: s.URL + "/prefix/v1/", Profiles: []string{descriptor.Profile}, Capabilities: []string{"streaming"}}, Auth: descriptor.Auth{Methods: []string{"none"}}}
	session, err := authentication.None{}.Resolve(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	c := Client{HTTP: transport.Client(time.Second), Base: d.API.BaseURL, Session: session}
	defer c.HTTP.CloseIdleConnections()
	catalog, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 {
		t.Fatal("incorrect catalog", catalog)
	}
	models := []string{catalog[0].ID, catalog[1].ID}
	if m, e := SelectModel(models, "", ""); e != nil || m != "a" {
		t.Fatal(m, e)
	}
	if _, e := SelectModel(models, "missing", ""); e == nil {
		t.Fatal("missing default accepted")
	}
	output, err := c.Chat(context.Background(), "a", []Message{{Role: "user", Content: "hi"}}, true, func(string) error { return nil })
	if err == nil || output != "partial" || calls != 1 {
		t.Fatal(output, err, calls)
	}
}

func TestTextCompletionPolicyAcrossResponseModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload string
			reject        bool
		}{
			{"text", `{"content":"Hello","tool_calls":null}`, false},
			{"text and tool call", `{"content":"Looking it up","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}`, true},
			{"tool call only", `{"content":null,"tool_calls":[{"id":"call_1"}]}`, true},
		} {
			t.Run(fmt.Sprintf("stream=%v/%s", stream, tc.name), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\ndata: [DONE]\n\n", tc.payload)
					} else {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{"choices":[{"message":%s}]}`, tc.payload)
					}
				}))
				defer server.Close()
				ctx := context.Background()
				session, err := (authentication.None{}).Resolve(ctx, descriptor.Descriptor{API: descriptor.API{BaseURL: server.URL}, Auth: descriptor.Auth{Methods: []string{"none"}}})
				if err != nil {
					t.Fatal(err)
				}
				client := Client{HTTP: transport.Client(time.Second), Base: server.URL, Session: session}
				defer client.HTTP.CloseIdleConnections()
				var emitted strings.Builder
				answer, err := client.Chat(ctx, "chat", []Message{{Role: "user", Content: "hi"}}, stream, func(s string) error { emitted.WriteString(s); return nil })
				if tc.reject {
					if err == nil || !strings.Contains(err.Error(), "unsupported tool-call") || emitted.Len() != 0 {
						t.Fatalf("unsupported payload accepted/emitted: %q (%v)", emitted.String(), err)
					}
				} else if err != nil || answer != "Hello" || emitted.String() != answer {
					t.Fatalf("text completion failed: %q (%v)", answer, err)
				}
			})
		}
	}
}

func FuzzStream(f *testing.F) {
	f.Add("data: [DONE]\n\n")
	f.Fuzz(func(t *testing.T, s string) { readStream(strings.NewReader(s), func(string) error { return nil }) })
}

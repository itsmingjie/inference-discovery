package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

func TestFallbackChatAndAuthOrder(t *testing.T) {
	method := "none"
	stream := true
	contextWindow := int64(131072)
	hits := 0
	var s *httptest.Server
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credential leak")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/descriptor":
			capabilities := []string{}
			if stream {
				capabilities = append(capabilities, "streaming")
			}
			json.NewEncoder(w).Encode(descriptor.Descriptor{Version: 1, Name: "Office AI", API: descriptor.API{BaseURL: s.URL + "/v1", Profiles: []string{descriptor.Profile}, Capabilities: capabilities, DefaultModel: "chat", Models: []descriptor.Model{{ID: "chat"}, {ID: "reasoner", ContextWindow: &contextWindow}}}, Auth: descriptor.Auth{Methods: []string{method}}})
		case "/v1/models":
			t.Error("client must use the descriptor catalog, not fetch /models")
			http.Error(w, "unexpected request", 500)
		case "/v1/chat/completions":
			hits++
			var body struct {
				Model  string
				Stream bool
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "reasoner" {
				t.Errorf("model override not sent: %q (%v)", body.Model, err)
			}
			if body.Stream != stream {
				t.Error("streaming mode did not follow the descriptor")
			}
			if stream {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello!\"}}]}\n\ndata: [DONE]\n\n")
			} else {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello!"}}]}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	var out, errout bytes.Buffer
	args := []string{"chat", "--descriptor-url", s.URL + "/descriptor", "--model", "reasoner"}
	if err := Run(context.Background(), args, strings.NewReader("hi\n/quit\n"), &out, &errout); err != nil {
		t.Fatal(err)
	}
	if hits != 1 || !strings.Contains(out.String(), "Hello!") || !strings.Contains(out.String(), "unencrypted") {
		t.Fatal(hits, out.String())
	}
	if err := Run(context.Background(), append(args, "--model", "missing"), strings.NewReader("hi\n"), &out, &errout); err == nil {
		t.Fatal("accepted model outside descriptor catalog")
	}
	if hits != 1 {
		t.Fatal("unavailable model reached endpoint")
	}
	stream = false
	out.Reset()
	if err := Run(context.Background(), args, strings.NewReader("hi\n/quit\n"), &out, &errout); err != nil {
		t.Fatal(err)
	}
	if hits != 2 || !strings.Contains(out.String(), "Hello!") {
		t.Fatal("non-streaming model needed extra configuration", hits, out.String())
	}
	if err := Run(context.Background(), []string{"inspect", "--descriptor-url", s.URL + "/descriptor"}, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal("non-streaming provider incorrectly reported as incompatible", err)
	}
	method = "approval"
	hits = 0
	if err := Run(context.Background(), args, strings.NewReader("hi\n"), &out, &errout); err == nil || !strings.Contains(err.Error(), "unsupported authentication") {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatal("endpoint accessed before auth rejection")
	}
	out.Reset()
	if err := Run(context.Background(), []string{"discover", "--json", "--descriptor-url", s.URL + "/descriptor"}, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatal(out.String())
	}
}

func TestMetadataHealthAndMethods(t *testing.T) {
	body := []byte(`{"version":1}`)
	var document atomic.Pointer[[]byte]
	document.Store(&body)
	h := metadataHandler(&document)
	for _, c := range []struct {
		method, path string
		status       int
	}{{"GET", descriptor.Path, 200}, {"POST", descriptor.Path, 405}, {"GET", "/v1/models", 404}, {"GET", descriptor.Path + "?x=y", 404}} {
		r := httptest.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatal(w.Code, c)
		}
	}
	document.Store(nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", descriptor.Path, nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

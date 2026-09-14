package inference

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func TestProbeRequiresValidFunctionArguments(t *testing.T) {
	for _, args := range []string{`{"ok":true}`, `{"OK":true}`, `{"ok":true,"extra":false}`, `{"ok":false,"ok":true}`} {
		stream := fmt.Sprintf("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"name\":\"inference_probe\",\"call_id\":\"test\",\"arguments\":%s}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", strconv.Quote(args))
		if valid := readProbe(strings.NewReader(stream), descriptor.ResponsesProfile, true) == nil; valid != (args == `{"ok":true}`) {
			t.Fatalf("unexpected argument validation for %s", args)
		}
	}
}

func TestProbeRejectsFalseEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, profile, stream string
		tools                 bool
	}{
		{"empty success", descriptor.Profile, "data: [DONE]\n\n", false},
		{"ignored tools", descriptor.Profile, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n", true},
		{"interrupted", descriptor.Profile, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n", false},
		{"failed response", descriptor.ResponsesProfile, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.failed\"}\n\n", false},
		{"incomplete response", descriptor.ResponsesProfile, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\"}}\n\n", false},
		{"malformed", descriptor.Profile, "data: {\n\n", false},
		{"oversized", descriptor.Profile, "data: " + strings.Repeat("x", 65537) + "\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if readProbe(strings.NewReader(tc.stream), tc.profile, tc.tools) == nil {
				t.Fatal("accepted false capability evidence")
			}
		})
	}
}

func TestDetectionDoesNotFollowRedirectsOrRetryFailures(t *testing.T) {
	var hits atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect"); http.Error(w, "no", 500) }))
	defer sink.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials sent")
		}
		if strings.HasSuffix(r.URL.Path, "/responses") {
			http.Error(w, "no", 401)
			return
		}
		w.Header().Set("Location", sink.URL)
		w.WriteHeader(307)
	}))
	defer server.Close()
	ctx := context.Background()
	session, err := (authentication.None{}).Resolve(ctx, descriptor.Descriptor{API: descriptor.API{BaseURL: server.URL}, Auth: descriptor.Auth{Methods: []string{"none"}}})
	if err != nil {
		t.Fatal(err)
	}
	httpClient := transport.Client(time.Second)
	defer httpClient.CloseIdleConnections()
	api := Client{HTTP: httpClient, Base: server.URL, Session: session}
	if _, err := api.Detect(ctx, "chat"); err == nil {
		t.Fatal("unconfirmed endpoint advertised")
	}
	if hits.Load() != 2 {
		t.Fatal("probe was retried", hits.Load())
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := api.Detect(ctx, "chat"); err == nil {
		t.Fatal("ignored cancellation")
	}
	if hits.Load() != 2 {
		t.Fatalf("request sent after cancellation: %d", hits.Load())
	}
}

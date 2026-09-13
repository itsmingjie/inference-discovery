package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/discovery"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

type recordedDiscovery []discovery.Record

func (r recordedDiscovery) Browse(context.Context, net.Interface) ([]discovery.Record, error) {
	return r, nil
}

func (recordedDiscovery) Advertise(context.Context, discovery.Advertisement) error {
	return fmt.Errorf("unexpected advertisement")
}

func TestResolveIsolatesSlowProviders(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":1,"name":"Healthy","api":{"base_url":"http://127.0.0.1:8000/v1","profiles":["openai-chat-completions"],"capabilities":[],"models":[{"id":"chat"}]},"auth":{"methods":["none"]}}`)
	}))
	defer healthy.Close()
	client := transport.Client(time.Second)
	defer client.CloseIdleConnections()
	records := recordedDiscovery{{ID: "slow", URLs: []string{slow.URL}}, {ID: "healthy", URLs: []string{healthy.URL}}}
	providers, err := resolve(context.Background(), records, net.Interface{}, client, options{timeout: time.Second}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 || providers[0].ID != "slow" || providers[1].ID != "healthy" {
		t.Fatalf("resolution changed browse order: %+v", providers)
	}
	if providers[0].Problem == "" || providers[1].Descriptor == nil {
		t.Fatalf("slow provider blocked the healthy provider: %+v", providers)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolve(ctx, records, net.Interface{}, client, options{timeout: time.Second}, io.Discard); err != context.Canceled {
		t.Fatalf("canceled scan reported providers instead of cancellation: %v", err)
	}
}

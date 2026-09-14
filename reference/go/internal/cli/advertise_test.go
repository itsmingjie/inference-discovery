package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/discovery"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

type observedDiscovery struct {
	discovery.Backend
	started chan discovery.Advertisement
	stopped chan struct{}
}

func (d observedDiscovery) Advertise(ctx context.Context, a discovery.Advertisement) error {
	d.started <- a
	<-ctx.Done()
	d.stopped <- struct{}{}
	return nil
}

func TestAdvertiserWaitsForEndpointAndRecovers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var healthy atomic.Bool
	checks := make(chan struct{}, 10)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"chat","api":{"profiles":["openai-chat-completions"],"capabilities":["streaming"]}}]}`)
		} else {
			http.Error(w, "starting", 503)
		}
		select {
		case checks <- struct{}{}:
		default:
		}
	}))
	defer endpoint.Close()
	session, err := (authentication.None{}).Resolve(ctx, descriptor.Descriptor{
		API: descriptor.API{BaseURL: endpoint.URL}, Auth: descriptor.Auth{Methods: []string{"none"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := transport.Client(time.Second)
	defer client.CloseIdleConnections()
	catalog := catalogBuilder{
		cfg: advertiseConfig{Name: "Office", Endpoint: endpoint.URL, Listen: "127.0.0.1:0"},
		api: inference.Client{HTTP: client, Base: endpoint.URL, Session: session}, out: io.Discard,
	}
	backend := observedDiscovery{started: make(chan discovery.Advertisement, 2), stopped: make(chan struct{}, 2)}
	done := make(chan error, 1)
	go func() {
		done <- catalog.advertise(ctx, net.Interface{Name: "test"}, 10*time.Millisecond, backend, io.Discard)
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		select {
		case <-checks:
		case <-ctx.Done():
			t.Fatal("advertiser did not retry the unavailable endpoint")
		}
	}
	select {
	case <-backend.started:
		t.Fatal("advertised before the endpoint was ready")
	default:
	}
	var location string
	for cycle := range 2 {
		healthy.Store(true)
		select {
		case a := <-backend.started:
			location = fmt.Sprintf("http://127.0.0.1:%d%s", a.Port, a.Path)
		case <-ctx.Done():
			t.Fatal("endpoint did not become discoverable")
		}
		response, err := client.Get(location)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("advertised unavailable metadata")
		}
		if cycle == 1 {
			break
		}
		healthy.Store(false)
		select {
		case <-backend.stopped:
		case <-ctx.Done():
			t.Fatal("unavailable endpoint was not withdrawn")
		}
		response, err = client.Get(location)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 503 {
			t.Fatal("unavailable endpoint still served a descriptor")
		}
	}
}

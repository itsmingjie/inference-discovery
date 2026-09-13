package transport

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNoAmbientCredentialsOrRedirects(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Proxy-Authorization") != "" {
			t.Error("credentials leaked")
		}
		w.Header().Set("Set-Cookie", "secret=1")
		http.Redirect(w, r, target.URL, 302)
	}))
	defer server.Close()
	for _, env := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		t.Setenv(env, target.URL)
	}
	t.Setenv("OPENAI_API_KEY", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	c := Client(time.Second)
	defer c.CloseIdleConnections()
	for i := 0; i < 2; i++ {
		if _, err := Request(context.Background(), c, "GET", server.URL, nil); err == nil {
			t.Fatal("accepted redirect")
		}
	}
	if targetHits.Load() != 0 {
		t.Fatal("redirect or proxy followed")
	}
}

func TestTLSValidation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer server.Close()
	untrusted := Client(5 * time.Second)
	defer untrusted.CloseIdleConnections()
	_, err := Request(context.Background(), untrusted, "GET", server.URL, nil)
	var certificateError x509.UnknownAuthorityError
	if !errors.As(err, &certificateError) {
		t.Fatalf("expected certificate rejection, got %v", err)
	}

	// Transport configuration is immutable once requests have started.
	trusted := Client(5 * time.Second)
	defer trusted.CloseIdleConnections()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	trusted.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	res, err := Request(context.Background(), trusted, "GET", server.URL, nil)
	if err != nil {
		t.Fatal("trusted HTTPS failed", err)
	}
	res.Body.Close()
}

func TestResponseBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, strings.Repeat("x", 100))
	}))
	defer server.Close()
	c := Client(5 * time.Second)
	defer c.CloseIdleConnections()
	res, err := Request(context.Background(), c, "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Read(res, 10, "application/json"); err == nil {
		t.Fatal("accepted oversized body")
	}
}

func TestRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	c := Client(30 * time.Millisecond)
	defer c.CloseIdleConnections()
	_, err := Request(context.Background(), c, "GET", server.URL, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected request deadline, got %v", err)
	}
}

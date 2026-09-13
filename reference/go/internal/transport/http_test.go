package transport

import (
	"context"
	"crypto/x509"
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

func TestTLSBoundsAndTimeout(t *testing.T) {
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer tls.Close()
	c := Client(30 * time.Millisecond)
	defer c.CloseIdleConnections()
	if _, err := Request(context.Background(), c, "GET", tls.URL, nil); err == nil {
		t.Fatal("accepted untrusted certificate")
	}
	roots := x509.NewCertPool()
	roots.AddCert(tls.Certificate())
	c.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	trusted, err := Request(context.Background(), c, "GET", tls.URL, nil)
	if err != nil {
		t.Fatal("trusted HTTPS failed", err)
	}
	trusted.Body.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, strings.Repeat("x", 100))
	}))
	defer server.Close()
	if _, err := Request(context.Background(), c, "GET", server.URL+"/slow", nil); err == nil {
		t.Fatal("timeout not applied")
	}
	res, err := Request(context.Background(), c, "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Read(res, 10, "application/json"); err == nil {
		t.Fatal("accepted oversized body")
	}
}

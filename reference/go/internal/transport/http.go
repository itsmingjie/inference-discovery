// Package transport constructs clients with no ambient credentials or redirects.
package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

func Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:    5 * time.Second,
			ResponseHeaderTimeout:  10 * time.Second,
			MaxResponseHeaderBytes: 16384,
			DisableCompression:     true,
			MaxIdleConns:           8,
			MaxIdleConnsPerHost:    2,
			IdleConnTimeout:        30 * time.Second,
		},
	}
}

func Request(ctx context.Context, c *http.Client, method, raw string, body io.Reader) (*http.Response, error) {
	return AuthorizedRequest(ctx, c, method, raw, body, nil)
}

func AuthorizedRequest(ctx context.Context, c *http.Client, method, raw string, body io.Reader, authorize func(*http.Request) error) (*http.Response, error) {
	if _, err := descriptor.URL(raw); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
	}
	if authorize != nil {
		if err := authorize(r); err != nil {
			return nil, err
		}
	}
	res, err := c.Do(r)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("HTTP %d from endpoint (redirects and authentication fallback disabled)", res.StatusCode)
	}
	return res, nil
}

func Read(res *http.Response, max int, kind string) ([]byte, error) {
	defer res.Body.Close()
	media, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || media != kind {
		return nil, fmt.Errorf("expected Content-Type %s", kind)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return b, nil
}

func Fetch(ctx context.Context, c *http.Client, raw string) (descriptor.Descriptor, []byte, error) {
	res, err := Request(ctx, c, "GET", raw, nil)
	if err != nil {
		return descriptor.Descriptor{}, nil, err
	}
	b, err := Read(res, descriptor.MaxBytes, "application/json")
	if err != nil {
		return descriptor.Descriptor{}, nil, err
	}
	d, err := descriptor.Parse(b)
	return d, b, err
}

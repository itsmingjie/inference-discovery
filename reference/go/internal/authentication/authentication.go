// Package authentication is the extension point for endpoint-scoped authentication.
package authentication

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

// Resolver runs after user selection and before any inference endpoint request.
// Future implementations must bind credentials to the exact selected endpoint.
type Resolver interface {
	Resolve(context.Context, descriptor.Descriptor) (Session, error)
}

type Session interface{ Authorize(*http.Request) error }

type None struct{}

type anonymous struct{ base string }

func (None) Resolve(_ context.Context, d descriptor.Descriptor) (Session, error) {
	// Descriptor validation belongs to the connection stage; authentication only
	// needs the explicit method and endpoint scope (also used during preflight).
	if _, err := descriptor.URL(d.API.BaseURL); err != nil {
		return nil, err
	}
	if len(d.Auth.Methods) != 1 || d.Auth.Methods[0] != "none" {
		return nil, fmt.Errorf("unsupported authentication: v0 accepts only [none]; no anonymous fallback")
	}
	return anonymous{d.API.BaseURL}, nil
}

func (a anonymous) Authorize(r *http.Request) error {
	u, err := descriptor.URL(a.base)
	if err != nil {
		return err
	}
	if r.URL.Scheme != u.Scheme || r.URL.Host != u.Host {
		return fmt.Errorf("authentication scope mismatch")
	}
	if !strings.HasPrefix(r.URL.Path, strings.TrimRight(u.Path, "/")+"/") {
		return fmt.Errorf("authentication endpoint path scope mismatch")
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Proxy-Authorization") != "" || r.URL.User != nil {
		return fmt.Errorf("anonymous session cannot supply credentials")
	}
	return nil
}

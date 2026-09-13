package authentication

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

func TestNoneAndScope(t *testing.T) {
	d := descriptor.Descriptor{Version: 1, Name: "Test", API: descriptor.API{BaseURL: "https://example.com/v1", Profiles: []string{descriptor.Profile}, Capabilities: []string{}}, Auth: descriptor.Auth{Methods: []string{"none"}}}
	session, err := (None{}).Resolve(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range [][]string{{"invitation"}, {"none", "approval"}, {"bearer"}, {}} {
		d.Auth.Methods = method
		if _, err = (None{}).Resolve(context.Background(), d); err == nil {
			t.Fatal("accepted", method)
		}
	}
	r, _ := http.NewRequest("GET", "https://example.com/v1/models", nil)
	if err = session.Authorize(r); err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "secret")
	if session.Authorize(r) == nil {
		t.Fatal("accepted credentials")
	}
	r.Header.Del("Authorization")
	r.Header.Set("Cookie", "secret")
	if session.Authorize(r) == nil {
		t.Fatal("accepted cookie")
	}
	for _, raw := range []string{"http://example.com/v1/models", "https://other.com/v1/models", "https://example.com/other/models"} {
		r, _ = http.NewRequest("GET", raw, nil)
		if session.Authorize(r) == nil {
			t.Fatal("accepted scope", raw)
		}
	}
}

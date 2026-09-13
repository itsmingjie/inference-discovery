package descriptor

import (
	"strings"
	"testing"
)

const valid = `{"version":1,"name":"Office AI","api":{"base_url":"http://192.0.2.20:8000/v1","profiles":["openai-chat-completions"],"capabilities":["streaming"],"default_model":"office-chat","models":[{"id":"office-chat"}]},"auth":{"methods":["none"]}}`

// These are protocol limits and ambiguity regressions, not JSON syntax tests.
func TestDescriptorLimits(t *testing.T) {
	cases := map[string]string{
		"body limit":            strings.Repeat(" ", MaxBytes) + valid,
		"extension depth":       strings.Replace(valid, `"version":1`, `"x":`+strings.Repeat("[", 18)+`0`+strings.Repeat("]", 18)+`,"version":1`, 1),
		"terminal control":      strings.Replace(valid, "Office AI", "bad\\u001b[31m", 1),
		"case-sensitive fields": strings.Replace(valid, `"name"`, `"NAME"`, 1),
		"rounded version":       strings.Replace(valid, `"version":1`, `"version":1.00000000000000000000001`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("accepted invalid descriptor")
			}
		})
	}
}

func TestUnknownMembersCannotOverrideKnownFields(t *testing.T) {
	input := strings.Replace(valid, `"version":1`, `"Version":99,"Name":"Impostor","version":1`, 1)
	d, err := Parse([]byte(input))
	if err != nil || d.Version != 1 || d.Name != "Office AI" {
		t.Fatal(d, err)
	}
}

func TestModelCatalogPolicy(t *testing.T) {
	for name, catalog := range map[string]string{
		"empty":                    `[]`,
		"duplicate IDs":            `[{"id":"office-chat"},{"id":"office-chat"}]`,
		"absent default":           `[{"id":"other"}]`,
		"effort without reasoning": `[{"id":"office-chat","reasoning_efforts":["high"]}]`,
		"unknown is not zero":      `[{"id":"office-chat","context_window":0}]`,
		"output exceeds context":   `[{"id":"office-chat","context_window":100,"max_output_tokens":101}]`,
		"fractional limit":         `[{"id":"office-chat","context_window":131072.5}]`,
		"rounded fraction":         `[{"id":"office-chat","context_window":131072.000000000000001}]`,
		"unbounded exponent":       `[{"id":"office-chat","context_window":1e1000000000}]`,
		"empty name":               `[{"id":"office-chat","name":""}]`,
		"null limit":               `[{"id":"office-chat","max_output_tokens":null}]`,
		"null property":            `[{"id":"office-chat","reasoning":null}]`,
		"case-sensitive ID":        `[{"ID":"office-chat"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(valid, `[{"id":"office-chat"}]`, catalog, 1)
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("accepted invalid catalog")
			}
		})
	}
	input := strings.Replace(valid, `{"id":"office-chat"}`, `{"id":"office-chat","ID":"other","Reasoning":true}`, 1)
	d, err := Parse([]byte(input))
	if err != nil || d.API.Models[0].ID != "office-chat" || d.API.Models[0].Reasoning != nil {
		t.Fatalf("unknown fields changed model properties: %+v (%v)", d.API.Models, err)
	}
}

func TestURL(t *testing.T) {
	for _, s := range []string{"http://192.0.2.1:8000/v1", "https://example.com/v1/", "http://[::1]:8000/v1", "http://[fe80::1%25en0]:8080/v1"} {
		if _, err := URL(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"file:///tmp/x", "http://u:p@example.com/v1", "http://example.com/v1?k=x", "http://example.com/v1?", "http://example.com/v1#", "//example.com", "http://example.com:99999", "http://example.com/../v1", "http://example.com/%2e%2e/v1", "http://example.com/%1b", "http://example.com\\@evil.com", "http://-bad.com", "http://example.com:"} {
		if _, err := URL(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	s, err := APIURL("http://example.com/prefix/v1/", "chat/completions")
	if err != nil || s != "http://example.com/prefix/v1/chat/completions" {
		t.Fatal(s, err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(valid))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		Parse(b)
	})
}

// Numeric notation is a protocol interoperability rule, not a Go type rule.
func TestDescriptorIntegerValues(t *testing.T) {
	input := strings.Replace(valid, `"version":1`, `"version":1e0`, 1)
	input = strings.Replace(input, `{"id":"office-chat"}`, `{"id":"office-chat","context_window":1.31072e5,"max_output_tokens":16384.0}`, 1)
	d, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	m := d.API.Models[0]
	if d.Version != 1 || m.ContextWindow == nil || *m.ContextWindow != 131072 || m.MaxOutputTokens == nil || *m.MaxOutputTokens != 16384 {
		t.Fatalf("integer values changed: %+v", d)
	}
}

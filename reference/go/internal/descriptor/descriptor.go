// Package descriptor implements the language-independent v1 descriptor contract.
package descriptor

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBytes = 32768
const Profile = "openai-chat-completions"
const Path = "/.well-known/inference.json"

type Descriptor struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	API     API    `json:"api"`
	Auth    Auth   `json:"auth"`
}

type API struct {
	BaseURL      string   `json:"base_url"`
	Profiles     []string `json:"profiles"`
	Capabilities []string `json:"capabilities"`
	DefaultModel string   `json:"default_model,omitempty"`
	Models       []Model  `json:"models"`
}

type Auth struct {
	Methods []string `json:"methods"`
}

func Text(s string, max int) bool {
	if len(s) == 0 || len(s) > max || !utf8.ValidString(s) || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

var token = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func tokens(ss []string, empty bool) bool {
	if ss == nil || len(ss) > 32 || (!empty && len(ss) == 0) {
		return false
	}
	seen := map[string]bool{}
	for _, s := range ss {
		if !token.MatchString(s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

func (d Descriptor) Validate() error {
	if d.Version != 1 {
		return fmt.Errorf("unsupported descriptor version %d", d.Version)
	}
	if !Text(d.Name, 128) {
		return fmt.Errorf("invalid provider name")
	}
	if _, err := URL(d.API.BaseURL); err != nil {
		return fmt.Errorf("invalid api.base_url: %w", err)
	}
	if !tokens(d.API.Profiles, false) {
		return fmt.Errorf("invalid api.profiles")
	}
	if !tokens(d.API.Capabilities, true) {
		return fmt.Errorf("invalid api.capabilities")
	}
	if err := ValidateModels(d.API.Models); err != nil {
		return err
	}
	if d.API.DefaultModel != "" && !slices.ContainsFunc(d.API.Models, func(m Model) bool { return m.ID == d.API.DefaultModel }) {
		return fmt.Errorf("api.default_model must name a model in api.models")
	}
	if !tokens(d.Auth.Methods, false) {
		return fmt.Errorf("invalid auth.methods")
	}
	return nil
}

func (d Descriptor) Compatible(stream bool) error {
	if !slices.Contains(d.API.Profiles, Profile) {
		return fmt.Errorf("unsupported API profiles: text Chat Completions required")
	}
	if stream && !slices.Contains(d.API.Capabilities, "streaming") {
		return fmt.Errorf("unsupported capability: streaming required (use --no-stream)")
	}
	return nil
}

// URL accepts only explicit HTTP(S) origins without embedded credentials or queries.
func URL(raw string) (*url.URL, error) {
	if !Text(raw, 2048) || strings.Contains(raw, `\`) {
		return nil, fmt.Errorf("invalid URL text")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("malformed URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Opaque != "" {
		return nil, fmt.Errorf("URL must be absolute HTTP(S), without userinfo, query or fragment")
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid URL port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("empty URL port")
	}
	host := u.Hostname()
	base := strings.Split(host, "%")[0]
	if net.ParseIP(base) == nil {
		if strings.Contains(host, "%") || len(host) > 253 {
			return nil, fmt.Errorf("invalid hostname")
		}
		for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, fmt.Errorf("invalid hostname")
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
					return nil, fmt.Errorf("invalid hostname")
				}
			}
		}
	}
	if strings.ContainsAny(u.Path, "\\\r\n\x00") || (u.Path != "" && !Text(u.Path, 2048)) {
		return nil, fmt.Errorf("invalid URL path")
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return nil, fmt.Errorf("dot segments forbidden")
		}
	}
	return u, nil
}

func APIURL(base, suffix string) (string, error) {
	u, err := URL(base)
	if err != nil {
		return "", err
	}
	return u.JoinPath(suffix).String(), nil
}

// Safe strips terminal controls from both provider claims and generated text.
func Safe(s string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) {
			return '\uFFFD'
		}
		return r
	}, s)
}

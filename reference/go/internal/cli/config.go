package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

type advertiseConfig struct {
	Models         []descriptor.Model `json:"models"`
	Hostname       string             `json:"hostname"`
	Name           string             `json:"name"`
	Endpoint       string             `json:"endpoint"`
	Interface      string             `json:"interface"`
	Listen         string             `json:"listen"`
	Model          string             `json:"model"`
	NoStream       bool               `json:"no_stream"`
	AllowLoopback  bool               `json:"allow_loopback"`
	Timeout        string             `json:"timeout"`
	HealthInterval string             `json:"health_interval"`
	TLSCert        string             `json:"tls_cert"`
	TLSKey         string             `json:"tls_key"`
}

func parseAdvertiseConfig(args []string, errout io.Writer) (advertiseConfig, error) {
	cfg := advertiseConfig{Name: "Inference", Listen: ":0", Timeout: "10s", HealthInterval: "15s"}
	configPath := ""
	flags := func() *flag.FlagSet {
		f := flag.NewFlagSet("advertise", flag.ContinueOnError)
		f.SetOutput(errout)
		f.StringVar(&configPath, "config", configPath, "optional JSON configuration file; flags override values")
		f.StringVar(&cfg.Name, "name", cfg.Name, "provider display name")
		f.StringVar(&cfg.Endpoint, "endpoint", cfg.Endpoint, "existing open Chat Completions API base URL")
		f.StringVar(&cfg.Interface, "interface", cfg.Interface, "multicast interface (required when ambiguous)")
		f.StringVar(&cfg.Hostname, "hostname", cfg.Hostname, "explicit unique mDNS host label (for descriptor TLS certificates)")
		f.StringVar(&cfg.Listen, "listen", cfg.Listen, "metadata listen address; default all addresses, ephemeral port")
		f.StringVar(&cfg.Model, "model", cfg.Model, "default model; otherwise first sorted model")
		f.StringVar(&cfg.Timeout, "timeout", cfg.Timeout, "health request timeout")
		f.StringVar(&cfg.HealthInterval, "health-interval", cfg.HealthInterval, "health polling interval")
		f.BoolVar(&cfg.NoStream, "no-stream", cfg.NoStream, "do not claim streaming support")
		f.BoolVar(&cfg.AllowLoopback, "allow-loopback", cfg.AllowLoopback, "allow a same-machine-only loopback endpoint")
		f.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "descriptor server TLS certificate PEM")
		f.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "descriptor server TLS key PEM")
		return f
	}
	defaults := cfg
	fs := flags()
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if configPath != "" {
		b, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, err
		}
		if len(b) > descriptor.MaxBytes {
			return cfg, fmt.Errorf("configuration exceeds 32 KiB")
		}
		if err = descriptor.CheckJSON(b); err != nil {
			return cfg, err
		}
		cfg = defaults
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&cfg); err != nil {
			return cfg, err
		}
		fs = flags()
		if err = fs.Parse(args); err != nil {
			return cfg, err
		}
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected positional arguments")
	}
	if cfg.Models != nil {
		if err := descriptor.ValidateModels(cfg.Models); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

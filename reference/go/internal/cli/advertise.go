package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/discovery"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func advertise(ctx context.Context, args []string, out, errout io.Writer) error {
	cfg, err := parseAdvertiseConfig(args, errout)
	if err != nil {
		return err
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil || timeout < 100*time.Millisecond || timeout > time.Minute {
		return fmt.Errorf("timeout must be between 100ms and 1m")
	}
	interval, err := time.ParseDuration(cfg.HealthInterval)
	if err != nil || interval < time.Second || interval > 10*time.Minute {
		return fmt.Errorf("health interval must be between 1s and 10m")
	}
	u, err := descriptor.URL(cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("--endpoint: %w", err)
	}
	if !cfg.AllowLoopback {
		ip := net.ParseIP(strings.Split(u.Hostname(), "%")[0])
		if strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback()) {
			return fmt.Errorf("loopback endpoint cannot be used by peers; bind your server to a LAN address or explicitly use --allow-loopback")
		}
	}
	if !descriptor.Text(cfg.Name, 63) {
		return fmt.Errorf("name must be 1..63 UTF-8 bytes without controls")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return fmt.Errorf("--tls-cert and --tls-key must be supplied together")
	}
	if cfg.Hostname != "" {
		h, e := descriptor.URL("http://" + cfg.Hostname)
		if e != nil || h.Host != cfg.Hostname || strings.ContainsAny(cfg.Hostname, ".:[%/") {
			return fmt.Errorf("hostname must be a single ASCII DNS label")
		}
	}
	if cfg.TLSCert != "" {
		if cfg.Hostname == "" {
			return fmt.Errorf("descriptor TLS requires --hostname matching the certificate's <hostname>.local SAN")
		}
		if _, err = tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey); err != nil {
			return fmt.Errorf("descriptor TLS certificate: %w", err)
		}
	}
	nic, err := discovery.Interface(cfg.Interface)
	if err != nil {
		return err
	}
	c := transport.Client(timeout)
	defer c.CloseIdleConnections()
	d := descriptor.Descriptor{API: descriptor.API{BaseURL: cfg.Endpoint}, Auth: descriptor.Auth{Methods: []string{"none"}}}
	session, err := authentication.None{}.Resolve(ctx, d)
	if err != nil {
		return err
	}
	api := inference.Client{HTTP: c, Base: cfg.Endpoint, Session: session}
	detectionTimeout, err := time.ParseDuration(cfg.DetectionTimeout)
	if err != nil || detectionTimeout < 100*time.Millisecond || detectionTimeout > 10*time.Minute {
		return fmt.Errorf("detection timeout must be between 100ms and 10m")
	}
	catalog := catalogBuilder{cfg: cfg, api: api, timeout: detectionTimeout, out: errout}
	return catalog.advertise(ctx, nic, interval, discovery.MDNS{}, out)
}

func (catalog *catalogBuilder) advertise(ctx context.Context, nic net.Interface, interval time.Duration, backend discovery.Backend, out io.Writer) error {
	cfg := catalog.cfg
	fmt.Fprintln(out, "Checking available models and API support...")
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	var document atomic.Pointer[[]byte]
	handler := metadataHandler(&document)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       15 * time.Second,
		MaxHeaderBytes:    8192,
	}
	serverErr := make(chan error, 1)
	go func() {
		if cfg.TLSCert != "" {
			serverErr <- server.ServeTLS(listener, cfg.TLSCert, cfg.TLSKey)
		} else {
			serverErr <- server.Serve(listener)
		}
	}()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	scheme := "http"
	if cfg.TLSCert != "" {
		scheme = "https"
	}
	var cancelAd context.CancelFunc
	var adDone chan error
	start := func() {
		adCtx, cancel := context.WithCancel(ctx)
		cancelAd = cancel
		adDone = make(chan error, 1)
		go func() {
			adDone <- backend.Advertise(adCtx, discovery.Advertisement{
				Name:      cfg.Name,
				Host:      cfg.Hostname,
				Port:      listener.Addr().(*net.TCPAddr).Port,
				Interface: nic,
				Transport: scheme,
				Path:      descriptor.Path,
			})
		}()
	}
	stop := func() {
		if cancelAd != nil {
			cancelAd()
			<-adDone
			cancelAd = nil
			adDone = nil
		}
	}
	defer stop()
	fmt.Fprintf(out, "Inference goes directly to %s. Open access; discovery does not establish identity or locality.\n", descriptor.Safe(cfg.Endpoint))
	// Check immediately, then wait between completed checks. An unavailable server
	// can start later; until then metadata returns 503 and nothing is advertised.
	timer := time.NewTimer(0)
	defer timer.Stop()
	lastProblem := ""
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverErr:
			return fmt.Errorf("descriptor server stopped: %w", err)
		case err := <-adDone:
			cancelAd = nil
			adDone = nil
			if err == nil && ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("discovery backend stopped: %v", err)
		case <-timer.C:
			b, healthErr := catalog.build(ctx)
			timer.Reset(interval)
			if healthErr != nil {
				document.Store(nil)
				stop()
				problem := descriptor.Safe(healthErr.Error())
				if problem != lastProblem {
					fmt.Fprintln(catalog.out, "Not advertising; will retry:", problem)
				}
				lastProblem = problem
				continue
			}
			document.Store(&b)
			lastProblem = ""
			if cancelAd == nil {
				start()
				fmt.Fprintf(out, "Advertising %s on %s; metadata port %d\n", descriptor.Safe(cfg.Name), nic.Name, listener.Addr().(*net.TCPAddr).Port)
			}
		}
	}
}

func metadataHandler(document *atomic.Pointer[[]byte]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path != descriptor.Path || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", 405)
			return
		}
		body := document.Load()
		if body == nil {
			http.Error(w, "endpoint unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			w.Write(*body)
		}
	})
}

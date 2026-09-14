package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/discovery"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

var Version = "dev"

const usage = `Inference Discovery
Usage: inference <command> [options]
  advertise   Advertise an existing endpoint
  discover    List providers (--json, --watch)
  inspect     Show a descriptor and compatibility
  chat        Select a provider and chat
Use inference <command> --help for options.
`

func Run(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		_, err := fmt.Fprint(out, usage)
		return err
	}
	if args[0] == "--version" {
		fmt.Fprintln(out, Version)
		return nil
	}
	if args[0] == "advertise" {
		err := advertise(ctx, args[1:], out, errout)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if args[0] != "discover" && args[0] != "inspect" && args[0] != "chat" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	opts := options{}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errout)
	fs.StringVar(&opts.iface, "interface", "", "network interface (required when ambiguous)")
	fs.StringVar(&opts.url, "descriptor-url", "", "explicit descriptor URL; bypass multicast")
	if args[0] != "discover" {
		fs.StringVar(&opts.provider, "provider", "", "preferred provider name or discovery ID")
	}
	if args[0] == "chat" {
		fs.StringVar(&opts.model, "model", "", "model override")
	}
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Second, "descriptor request timeout")
	fs.DurationVar(&opts.browse, "browse-timeout", 3*time.Second, "fresh discovery snapshot duration")
	opts.chatTimeout = 2 * time.Minute
	if args[0] == "chat" {
		fs.DurationVar(&opts.chatTimeout, "chat-timeout", opts.chatTimeout, "maximum time for one inference request")
	}
	if args[0] == "discover" {
		fs.BoolVar(&opts.json, "json", false, "JSON output (watch uses JSON Lines)")
		fs.BoolVar(&opts.watch, "watch", false, "watch fresh snapshots for added, updated, removed providers")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	for _, d := range []time.Duration{opts.timeout, opts.browse, opts.chatTimeout} {
		if d < 100*time.Millisecond || d > 10*time.Minute {
			return fmt.Errorf("timeouts must be between 100ms and 10m")
		}
	}
	client := transport.Client(opts.timeout)
	defer client.CloseIdleConnections()
	var nic net.Interface
	var err error
	if opts.url == "" {
		nic, err = discovery.Interface(opts.iface)
		if err != nil {
			return err
		}
	} else {
		if _, err = descriptor.URL(opts.url); err != nil {
			return err
		}
	}
	scan := func() ([]provider, error) { return resolve(ctx, discovery.MDNS{}, nic, client, opts, errout) }
	if args[0] == "discover" {
		return discover(ctx, scan, opts, out)
	}
	providers, err := scan()
	if err != nil {
		return err
	}
	reader := bufio.NewScanner(in)
	reader.Buffer(make([]byte, 4096), 65536)
	selectionOutput := out
	if args[0] == "inspect" {
		selectionOutput = errout
	}
	selected, err := selectProvider(providers, opts.provider, reader, selectionOutput)
	if err != nil {
		return err
	}
	if args[0] == "inspect" {
		return inspect(ctx, selected, out)
	}
	return chat(ctx, selected, opts, reader, out)
}

type options struct {
	iface, url, provider, model  string
	timeout, browse, chatTimeout time.Duration
	json, watch                  bool
}

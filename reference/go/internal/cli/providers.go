package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/discovery"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

type provider struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	URL        string                 `json:"descriptor_url,omitempty"`
	Descriptor *descriptor.Descriptor `json:"descriptor,omitempty"`
	Problem    string                 `json:"error,omitempty"`
	Raw        []byte                 `json:"-"`
}

func resolve(ctx context.Context, backend discovery.Backend, nic net.Interface, c *http.Client, o options, errout io.Writer) ([]provider, error) {
	records := []discovery.Record{{ID: o.url, URLs: []string{o.url}}}
	if o.url == "" {
		var err error
		browseCtx, cancel := context.WithTimeout(ctx, o.browse)
		records, err = backend.Browse(browseCtx, nic)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	if len(records) > 64 {
		fmt.Fprintln(errout, "Provider limit: considering first 64 records")
		records = records[:64]
	}
	// Bound the whole descriptor pass as well as individual requests.
	pass, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	// All admitted providers start together: an unreachable endpoint cannot
	// consume another provider's chance to resolve. Results retain browse order.
	result := make([]provider, len(records))
	var workers sync.WaitGroup
	for i, record := range records {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result[i] = resolveProvider(pass, record, c)
		}()
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func resolveProvider(ctx context.Context, record discovery.Record, client *http.Client) provider {
	p := provider{ID: record.ID, Name: record.Name, Problem: record.Problem}
	if p.Problem != "" {
		return p
	}
	p.Problem = "no usable descriptor"
	for _, raw := range record.URLs {
		p.URL = raw
		d, b, err := transport.Fetch(ctx, client, raw)
		if err != nil {
			p.Problem = err.Error()
			continue
		}
		p.Name, p.Descriptor, p.Raw, p.Problem = d.Name, &d, b, ""
		break
	}
	return p
}

func display(out io.Writer, p provider, prefix string) {
	if p.Descriptor == nil {
		fmt.Fprintf(out, "%s%s: unavailable (%s)\n", prefix, descriptor.Safe(p.Name), descriptor.Safe(p.Problem))
		return
	}
	encryption := "HTTP, unencrypted"
	if strings.HasPrefix(p.Descriptor.API.BaseURL, "https:") {
		encryption = "HTTPS, encrypted"
	}
	fmt.Fprintf(out, "%s%s — %s [%s]\n", prefix, descriptor.Safe(p.Name), descriptor.Safe(p.Descriptor.API.BaseURL), encryption)
	if strings.HasPrefix(p.URL, "http:") {
		fmt.Fprintln(out, "     Descriptor: HTTP, unencrypted")
	}
}

func selectProvider(ps []provider, preferred string, in *bufio.Scanner, out io.Writer) (provider, error) {
	if len(ps) == 0 {
		return provider{}, fmt.Errorf("no providers found; check interface or use --descriptor-url")
	}
	if preferred != "" {
		matches := []provider{}
		for _, p := range ps {
			if p.Name == preferred || p.ID == preferred {
				matches = append(matches, p)
			}
		}
		if len(matches) != 1 {
			return provider{}, fmt.Errorf("preferred provider must match exactly one name or ID")
		}
		p := matches[0]
		display(out, p, "")
		if p.Descriptor == nil {
			return p, fmt.Errorf("provider unavailable: %s", p.Problem)
		}
		return p, nil
	}
	fmt.Fprintln(out, "Available inference:")
	for i, p := range ps {
		display(out, p, fmt.Sprintf("  %d. ", i+1))
	}
	choice := 1
	if len(ps) > 1 {
		fmt.Fprint(out, "Choose a provider: ")
		if !in.Scan() {
			return provider{}, fmt.Errorf("provider selection required (or use --provider)")
		}
		var err error
		choice, err = strconv.Atoi(strings.TrimSpace(in.Text()))
		if err != nil || choice < 1 || choice > len(ps) {
			return provider{}, fmt.Errorf("invalid provider selection")
		}
	}
	p := ps[choice-1]
	if p.Descriptor == nil {
		return p, fmt.Errorf("provider unavailable: %s", p.Problem)
	}
	return p, nil
}

// Package discovery isolates replaceable mDNS libraries from protocol and CLI code.
package discovery

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/brutella/dnssd"
	"github.com/grandcat/zeroconf"
	"github.com/libp2p/go-netroute"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

const Service = "_inference._tcp"

type Record struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URLs    []string `json:"descriptor_urls"`
	Problem string   `json:"error,omitempty"`
}

type Advertisement struct {
	Name      string
	Host      string
	Port      int
	Interface net.Interface
	Transport string
	Path      string
}

type Backend interface {
	Advertise(context.Context, Advertisement) error
	Browse(context.Context, net.Interface) ([]Record, error)
}

type MDNS struct{}

// Interface inspects local interface configuration only; it sends no probes.
func Interface(name string) (net.Interface, error) {
	if name != "" {
		nic, err := net.InterfaceByName(name)
		if err != nil {
			return net.Interface{}, err
		}
		if nic.Flags&net.FlagUp == 0 || nic.Flags&net.FlagMulticast == 0 {
			return net.Interface{}, fmt.Errorf("interface must be up and multicast-capable")
		}
		return *nic, nil
	}
	nics, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	candidates := []net.Interface{}
	for _, nic := range nics {
		if nic.Flags&net.FlagUp == 0 || nic.Flags&net.FlagMulticast == 0 || nic.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addrs, _ := nic.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip != nil && ip.IsGlobalUnicast() {
				candidates = append(candidates, nic)
				break
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	// Ask the OS where mDNS would go, without contacting anything on the network.
	// Never broaden discovery to every interface or select an ineligible VPN route.
	if len(candidates) > 1 {
		if router, err := netroute.New(); err == nil {
			for _, destination := range []net.IP{net.IPv4(224, 0, 0, 251), net.ParseIP("ff02::fb")} {
				routed, _, _, err := router.Route(destination)
				if err != nil || routed == nil {
					continue
				}
				for _, nic := range candidates {
					if nic.Index == routed.Index {
						return nic, nil
					}
				}
			}
		}
	}
	names := []string{}
	for _, n := range candidates {
		names = append(names, n.Name)
	}
	return net.Interface{}, fmt.Errorf("select --interface explicitly (eligible: %s)", strings.Join(names, ", "))
}

func (MDNS) Advertise(ctx context.Context, a Advertisement) error {
	if !descriptor.Text(a.Name, 63) {
		return fmt.Errorf("service name must be 1..63 UTF-8 bytes without controls")
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	id := hex.EncodeToString(nonce)
	// Duplicate display names are normal. Give each registration its own DNS label
	// rather than relying on all peers receiving collision probes. Clients display
	// the descriptor name, which remains unchanged.
	name := a.Name
	if len(name) > 46 {
		name = strings.ToValidUTF8(name[:46], "")
	}
	name += "-" + id
	host := a.Host
	if host == "" {
		host = "inference-" + id
	}
	s, err := dnssd.NewService(dnssd.Config{
		Name:   name,
		Host:   host,
		Type:   Service,
		Port:   a.Port,
		Ifaces: []string{a.Interface.Name},
		Text:   map[string]string{"v": "1", "transport": a.Transport, "path": a.Path},
	})
	if err != nil {
		return err
	}
	r, err := dnssd.NewResponder()
	if err != nil {
		return err
	}
	if _, err = r.Add(s); err != nil {
		return err
	}
	err = r.Respond(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (MDNS) Browse(ctx context.Context, nic net.Interface) ([]Record, error) {
	r, err := zeroconf.NewResolver(zeroconf.SelectIfaces([]net.Interface{nic}))
	if err != nil {
		return nil, err
	}
	// Drain until closure even after cancellation: upstream sends synchronously.
	entries := make(chan *zeroconf.ServiceEntry, 32)
	if err = r.Browse(ctx, Service, "local.", entries); err != nil {
		return nil, err
	}
	records := map[string]Record{}
	for e := range entries {
		if len(records) >= 256 {
			continue
		}
		rec := Record{ID: e.ServiceInstanceName(), Name: unescape(e.Instance)}
		if !descriptor.Text(rec.Name, 63) {
			rec.Name = fmt.Sprintf("%q", rec.Name)
			rec.Problem = "invalid service instance name"
			records[rec.ID] = rec
			continue
		}
		rec.URLs, err = descriptorURLs(e.HostName, e.Port, e.Text, e.AddrIPv4, e.AddrIPv6, nic.Name)
		if err != nil {
			rec.Problem = err.Error()
		}
		records[rec.ID] = rec
	}
	result := slices.Collect(maps.Values(records))
	slices.SortFunc(result, func(a, b Record) int { return cmp.Compare(a.ID, b.ID) })
	return result, nil
}

// descriptorURLs constructs descriptor locations solely from SRV/address records and TXT.
func descriptorURLs(host string, port int, txt []string, v4, v6 []net.IP, iface string) ([]string, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SRV port")
	}
	values := map[string]string{}
	size := 0
	for _, s := range txt {
		size += len(s) + 1
		if len(s) > 255 || size > 512 {
			return nil, fmt.Errorf("TXT exceeds protocol limits")
		}
		key, val, ok := strings.Cut(s, "=")
		key = strings.ToLower(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("malformed TXT")
		}
		for _, c := range key {
			if c < 0x21 || c > 0x7e {
				return nil, fmt.Errorf("TXT keys must be printable ASCII")
			}
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate TXT key")
		}
		values[key] = val
	}
	if values["v"] != "1" {
		return nil, fmt.Errorf("unsupported or missing discovery version")
	}
	transport, path := values["transport"], values["path"]
	if transport != "http" && transport != "https" {
		return nil, fmt.Errorf("unsupported descriptor transport")
	}
	if len(path) == 0 || len(path) > 128 || path[0] != '/' || strings.HasPrefix(path, "//") {
		return nil, fmt.Errorf("invalid descriptor path")
	}
	test, err := descriptor.URL("http://example.invalid" + path)
	if err != nil || test.Host != "example.invalid" || test.EscapedPath() != path {
		return nil, fmt.Errorf("invalid descriptor path")
	}
	addresses := []string{}
	if transport == "https" {
		addresses = append(addresses, strings.TrimSuffix(host, "."))
	} else {
		for _, ip := range slices.Concat(v4, v6) {
			if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}
			s := ip.String()
			if ip.IsLinkLocalUnicast() && ip.To4() == nil {
				s += "%" + iface
			}
			addresses = append(addresses, s)
		}
	}
	urls := []string{}
	seen := map[string]bool{}
	for _, addr := range addresses {
		u := &url.URL{Scheme: transport, Host: net.JoinHostPort(addr, strconv.Itoa(port)), Path: test.Path, RawPath: test.RawPath}
		raw := u.String()
		if _, err = descriptor.URL(raw); err != nil {
			continue
		}
		if !seen[raw] {
			urls = append(urls, raw)
			seen[raw] = true
		}
		if len(urls) == 8 {
			break
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no usable descriptor address")
	}
	return urls, nil
}

func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if i+3 < len(s) {
				if n, e := strconv.Atoi(s[i+1 : i+4]); e == nil && n <= 255 {
					b.WriteByte(byte(n))
					i += 3
					continue
				}
			}
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

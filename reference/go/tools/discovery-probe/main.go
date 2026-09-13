// discovery-probe checks DNS-SD interoperability outside ordinary CI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/brutella/dnssd"
	"github.com/grandcat/zeroconf"
)

func main() {
	mode := flag.String("mode", "snapshot", "snapshot or advertise")
	ipVersion := flag.String("ip-version", "any", "snapshot transport: any, 4 or 6")
	iface := flag.String("interface", "", "required multicast interface")
	name := flag.String("name", "Inference Probe", "instance name (repeat to test collisions)")
	duration := flag.Duration("duration", 15*time.Second, "probe duration")
	flag.Parse()
	if *iface == "" {
		fmt.Fprintln(os.Stderr, "--interface is required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()
	var err error
	switch *mode {
	case "snapshot":
		var nic *net.Interface
		nic, err = net.InterfaceByName(*iface)
		if err != nil {
			break
		}
		var r *zeroconf.Resolver
		traffic := zeroconf.IPType(zeroconf.IPv4AndIPv6)
		switch *ipVersion {
		case "4":
			traffic = zeroconf.IPv4
		case "6":
			traffic = zeroconf.IPv6
		case "any":
		default:
			err = fmt.Errorf("ip-version must be any, 4 or 6")
			break
		}
		if err != nil {
			break
		}
		r, err = zeroconf.NewResolver(zeroconf.SelectIfaces([]net.Interface{*nic}), zeroconf.SelectIPTraffic(traffic))
		if err != nil {
			break
		}
		ch := make(chan *zeroconf.ServiceEntry)
		err = r.Browse(ctx, "_inference._tcp", "local.", ch)
		if err != nil {
			break
		}
		for e := range ch {
			b, _ := json.Marshal(e)
			fmt.Println(string(b))
		}
	case "advertise":
		var r dnssd.Responder
		r, err = dnssd.NewResponder()
		if err != nil {
			break
		}
		var s dnssd.Service
		s, err = dnssd.NewService(dnssd.Config{Name: *name, Host: "inference-probe-" + strconv.Itoa(os.Getpid()), Type: "_inference._tcp", Port: 18080, Ifaces: []string{*iface}, Text: map[string]string{"v": "1", "transport": "http", "path": "/descriptor.json"}})
		if err != nil {
			break
		}
		var h dnssd.ServiceHandle
		h, err = r.Add(s)
		if err != nil {
			break
		}
		go func() {
			select {
			case <-time.After(*duration / 2):
				h.UpdateText(map[string]string{"v": "1", "transport": "http", "path": "/updated.json"}, r)
				fmt.Println("TXT update sent")
			case <-ctx.Done():
			}
		}()
		err = r.Respond(ctx)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

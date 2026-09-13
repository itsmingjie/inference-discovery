# Explicit network interoperability tests

These tests send multicast and publish temporary services. Run deliberately on a
development LAN, outside ordinary CI. Do not substitute local processes for the
two-machine release exercise. Record versions, topology, commands and results.

Build the reference binary and the discovery probe:

```sh
cd reference/go
go build -o ../../bin/inference ./cmd/inference
go build -o ../../bin/discovery-probe ./tools/discovery-probe
cd ../..
```

## Bonjour on macOS

In separate terminals, using your selected interface:

```sh
dns-sd -B _inference._tcp local.
./bin/discovery-probe --mode advertise --interface en0 --duration 30s
dns-sd -L "Inference Probe" _inference._tcp local.
```

The probe changes TXT `path` halfway through and sends a goodbye at exit. Confirm
that resolution updates and the browser reports removal. Run a second identical
probe to check collision renaming. Then test the reverse direction:

```sh
dns-sd -R "Bonjour Probe" _inference._tcp local. 18081 v=1 transport=http path=/descriptor.json
./bin/discovery-probe --mode snapshot --interface en0 --duration 5s
```

The probe advertises metadata locations only; it does not serve HTTP. Use the
reference advertiser for an actual connection. Snapshot mode uses the same
browser as the reference CLI.

## Avahi on Linux

Install your distribution's `avahi-daemon` and `avahi-utils`, start the daemon,
and allow multicast on the chosen interface. In separate terminals:

```sh
avahi-browse -r _inference._tcp
./bin/discovery-probe --mode advertise --interface eth0 --duration 30s
avahi-publish-service "Avahi Probe" _inference._tcp 18081 v=1 transport=http path=/descriptor.json
./bin/discovery-probe --mode snapshot --interface eth0 --duration 5s
```

Verify Go → Avahi and Avahi → Go, TXT updates, same-name registrations, orderly
goodbyes, and abrupt-process disappearance in `inference discover --watch`.
Repeat across macOS and Linux hosts using the real reference advertiser and chat.

## Interfaces, address families, and loss

Repeat on each explicit interface. On two isolated links, verify an advertiser
selected for one link is not usable from the other unless routing intentionally
allows it. Two NICs on the same broadcast LAN do not establish this property.
Test IPv4-only, IPv6-only, dual stack, and IPv6 link-local scoped resolution.
Confirm metadata retrieval over each published reachable address and inference
over a matching endpoint URL. Use `dns-sd -G v4v6 HOST.local` to inspect addresses.

The snapshot probe can force multicast traffic onto one address family:

```sh
./bin/discovery-probe --mode snapshot --interface en0 --ip-version 4 --duration 5s
./bin/discovery-probe --mode snapshot --interface en0 --ip-version 6 --duration 5s
```

For a repeatable same-machine reference CLI smoke test, build the mock binary
with `go build -o bin/mock examples/local-provider/mock.go`, then run
`python3 scripts/network-smoke.py --interface en0`. It checks duplicate providers,
model selection, streamed mock text, IPv6 metadata, health withdrawal/503,
recovery, and clean removal. It intentionally uses a loopback inference API and
does not count as the two-machine acceptance walkthrough.

Unplug/reconnect the interface and restart the macOS advertiser if needed. Test
blocked multicast with explicit `--descriptor-url`. Test abrupt API loss,
advertiser termination, lost packets, duplicate display names, and default model
removal. No interrupted chat may retry or migrate. Record snapshot false negatives
and fragmented-record failures as defects, not successful conformance.

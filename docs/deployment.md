# Deployment

The advertiser is a metadata sidecar for an existing open API. It does not start
models, proxy requests, provide accounts, or turn an authenticated upstream into
an anonymous one. For network-scoped access, bind the API and metadata server to LAN addresses
and restrict their ports to the intended network with a firewall. Discovery does
not enforce physical presence: routed networks, VPNs, and public endpoints can
remain reachable from elsewhere.

## Interface and address selection

The default selects the sole eligible interface, or asks the OS which eligible
interface routes mDNS when several exist. It reads local configuration without
sending probes or scanning services. Loopback and point-to-point interfaces are
excluded from automatic selection. If the OS cannot resolve the choice, use
`--interface` to name one active multicast interface. Linux
names may be `eth0` or `enp...`; macOS often uses `en0`. A VPN, multiple NICs, Wi-Fi
client isolation, multicast filtering, or firewall may prevent discovery.

Metadata listens on `:0` by default (all local addresses, ephemeral TCP port),
independently of the multicast interface. Use `--listen :8081` for a stable
fallback URL, or a specific bind address. A specific bind address must match the
addresses clients resolve; an IPv4-only listener cannot serve an advertised IPv6
address. A firewall, not DNS-SD interface selection, enforces inbound isolation.

Allow UDP 5353 on the intended LAN and the metadata/API TCP ports. Do not relay
multicast across trust boundaries without understanding the exposure. HTTP
descriptor resolution uses IPv4 then IPv6 addresses, with a receiving-interface
zone for link-local IPv6. HTTPS uses the advertised SRV hostname and requires a
trusted certificate for it. The registrar generates a fresh
`inference-<random>.local` hostname to avoid fighting Bonjour over the system
hostname. For descriptor TLS, use `--hostname inference-office` and provision a
trusted certificate with the `inference-office.local` subject alternative name.
Ensure that label is unique on the network; a collision rename will correctly
cause TLS verification to fail. Alternatively serve the descriptor on an existing
HTTPS host and use the explicit descriptor-URL fallback.

DNS service instance names include a random suffix so providers with identical
display names coexist without operator coordination. Clients show the friendly
name from the descriptor. Instance IDs may change when a provider reappears;
they are not persistent identities.

An API base such as `http://127.0.0.1:8000/v1` points to each client's own machine.
The advertiser rejects literal loopback and `localhost` by default; hostname
aliases are not a complete loopback/SSRF defense. `--allow-loopback` is only for
same-machine development checks. Use a LAN-reachable API root for sharing.

## Configuration

`advertise --config FILE` loads a JSON object; command-line flags override its
values. Unknown config keys, including model properties, are errors. Config files must not contain keys or
credentials. Durations are Go duration strings such as `10s` and `1m`.

```json
{
  "name": "Office AI",
  "endpoint": "http://192.168.1.20:8000/v1"
}
```

Optional `tls_cert` and `tls_key` configure descriptor TLS only; the endpoint URL
independently controls inference TLS. There is no certificate validation bypass.
Config changes require a restart. The advertiser waits if the endpoint is still
starting. During health failures, metadata returns 503 and any advertisement is
withdrawn. A successful check updates the default model
and model catalog, and reannounces if necessary. An explicit `--model` never silently changes.

Use [model configuration](models.md) to publish model properties or restrict the
catalog to text models.

Capability detection is automatic; [model configuration](models.md) documents
its request limits and the optional overrides for skipping generation checks.

Run the advertiser under an existing process supervisor for unattended use.
SIGINT/SIGTERM sends goodbyes; forced termination relies on cache expiry or fresh
snapshots. On macOS the chosen registration library does not monitor link changes;
restart after changing networks. Explicit interface changes also require restart.

## Remote endpoints and extensions

For an already open remote endpoint, pass its HTTPS base URL directly. A local
advertiser is still required. For authenticated APIs, provision an existing
operator-managed gateway first. `none` clients send no keys and cannot connect
directly to authenticated OpenRouter or similar APIs. A key in an environment
variable, URL, or config will not make this reference authenticate upstream.

Future authentication can implement `authentication.Resolver` and a scoped session
at the existing connection stage. Unsupported methods currently fail closed,
including mixed lists such as `["none", "invitation"]`. Enrollment mechanisms
remain an extension point.

# Security boundary

DNS-SD/mDNS is an unauthenticated locator. Any participant on the discovery
network can claim a provider name, capability, host, or descriptor. There is no
provider identity, ownership proof, enrollment, or access control in v0.

Open access means anyone who can reach the inference endpoint can use it,
including people who never use discovery. Withdrawing an advertisement does not
revoke access. Operators must independently control exposure, spending and model
server capacity. A local advertisement can send prompts to a remote service. For network-scoped
access, restrict the API and descriptor ports to the intended LAN with bind
addresses and firewall rules. Wi-Fi discovery alone cannot prove physical presence;
routing and VPNs can extend network reachability.

## Transport and credentials

HTTP is allowed for LAN demonstrations. Both metadata and prompts can be read or
altered on unencrypted transports. An HTTPS inference endpoint does not protect
an HTTP descriptor from replacement. The CLI displays the API encryption state
and notes when descriptor transport is unencrypted. HTTPS uses normal certificate
and hostname validation. There is no insecure TLS flag.

The reference HTTP client starts with an empty credential context: no cookies,
environment proxies, `.netrc`, SDK keys, ambient bearer headers or client
certificates. It never follows redirects. URLs with userinfo or query parameters
are rejected, including explicit fallback URLs. HTTP response bodies from failed
requests are not printed. The `none` session rejects any authorization or cookie
header supplied by a caller.

Upstream API keys remain the operator's responsibility. A remote authenticated
API cannot be made anonymously usable merely by advertising its URL. An existing
operator-managed gateway may expose an open API; implementing that gateway is
outside this discovery reference. Future credential methods must explicitly
bind credentials to the selected endpoint's scheme, host, port and path scope,
and must fail closed on authentication errors. Discovery records never carry keys.

## Untrusted input and network bounds

Names, model IDs, capabilities, descriptors and generated text are untrusted.
Validate URL syntax, exact required fields, size, depth, duplicates, types,
versions and authentication methods before connecting. Escape or remove terminal
controls in display output. Cap descriptor bodies at 32 KiB, headers at 16 KiB,
models/completions at 1 MiB, and streaming events and duration as described in the
protocol. The metadata server bounds HTTP headers and read/write time.

This protocol intentionally permits LAN/private addresses: it cannot categorically
block private-address requests without defeating its purpose. An attacker can
advertise a URL for another reachable host. Descriptor requests are credential-free
GETs; inference requests happen only after provider selection and auth
resolution. Advertisers poll their operator-configured endpoint for model IDs. That does not make arbitrary internal services safe. Use a trusted
development network or an isolated VLAN; do not expose the CLI as an untrusted
multi-tenant server-side URL fetch service.

The discovery libraries are not designed to
resist unlimited malicious multicast traffic. The adapter caps visible providers
and descriptor passes, but upstream mDNS parsing and internal caches are not a
complete resource-exhaustion defense. Interface selection restricts active browsing
and advertisement targets; the registrar may open multicast sockets on other
interfaces. This is not a firewall or a hardened network isolation guarantee.

Do not automatically retry interrupted inference, migrate conversation history,
or use a different provider after an error. A fresh explicit connection is needed.
Capabilities and `/models` are claims; a successful real completion must be tested
separately. Health polling does not revoke access or prove generation readiness.

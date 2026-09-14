# Inference Discovery for Pi

Available network models appear in Pi's normal `/model` picker. The extension
maintains the provider catalog; Pi owns selection, inference and conversations.

## Install

Requires Node.js 22.18+ and Pi 0.85.1 from `@earendil-works/pi-coding-agent`.
The older `@mariozechner/pi-coding-agent` package is not supported.

From a source checkout (requires Go 1.24+) or an extracted release archive:

```sh
make install-pi
```

The installer prepares dependencies and registers the local package with Pi.
Keep this directory in place; the extension uses its bundled CLI automatically.
Use Pi as usual and open `/model`. The selected entry's model name includes the
provider, endpoint and whether transport is encrypted. Several providers can
offer the same model ID without colliding. The extension does not choose the
advertised default or change an active conversation.

On the provider's machine, advertise an open server that supports streaming and
function calling:

```sh
inference advertise --name "Office AI" --endpoint http://192.168.1.20:8000/v1
```

The advertiser detects each model's APIs and tool support. Pi chooses Responses
when available, otherwise Chat Completions with function tools. Models without
confirmed streaming and coding support are omitted from Pi's picker; they can
still be used with `inference chat` when text Chat Completions is available.
See [model configuration](../../docs/models.md) for optional manual overrides.

## Optional overrides

No client endpoint or model configuration is needed for multicast discovery.
To override automatic interface selection, set `INFERENCE_INTERFACE` to the
desired interface, such as `en0`. When multicast is blocked, set
`INFERENCE_DESCRIPTOR_URL` to an explicit descriptor URL; it takes precedence
over multicast. These variables must be present in Pi's environment.

The extension takes an initial snapshot before Pi finishes loading it, then
scans again five seconds after each completed snapshot. A scan is bounded to
15 seconds. Changed catalogs replace their previous registrations; unavailable
providers are removed. If discovery fails, existing discovery registrations are
cleared and scanning continues. Reappearance adds the models back. An existing
conversation is never moved to another provider.

## Model properties and security

Model IDs, reasoning support, recognized effort levels, context windows and
output limits come from the descriptor. Pi requires numeric limits, so omitted
limits use an 8,192-token context (or the advertised output limit if larger) and
a 1,024-token output limit capped by the context window. These are adapter
defaults, not verified server limits; advertise real limits for accurate context
management. Unadvertised effort levels are not enabled. Pi's required cost
bookkeeping uses zero values; no pricing is discovered or estimated.

The Go CLI validates descriptors and handles DNS-SD. Inference uses Pi's existing
API codecs with a direct HTTP transport: no credentials or cookies, no environment
proxy, no redirects, normal HTTPS certificate validation, and no transport
retries. Responses are limited to 16 MiB and two minutes. Pi's conversation,
tool-approval and higher-level retry policies remain under the user's control.
Discovery does not establish provider identity or guarantee local processing.

Node's URL parser does not support IPv6 zone identifiers in API URLs. Use a
hostname or an IPv4/global IPv6 address for those endpoints.

No configuration files are written by the extension and no relay is involved.
See the [protocol security boundary](../../spec/security.md).

## Development

From the repository root, build `inference`, then:

```sh
cd integrations/pi
npm ci
npm run check
npm test
```

Tests exercise Pi's native registry and package loader against the real discovery
CLI, plus streaming and credential isolation against local HTTP servers. They
do not require multicast or an upstream API key. Cross-machine discovery is
covered separately by the [network checks](../../docs/network-interop.md).

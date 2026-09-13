# Inference Discovery v0 — descriptor version 1

The key words MUST, MUST NOT, SHOULD and MAY describe interoperability requirements.
This contract is language independent. The Go implementation is a reference, not
the definition of the protocol.

## Scope and topology

The first profile enables text Chat Completions on an existing open endpoint.
A local advertiser serves a descriptor and publishes a DNS-SD service. Clients
retrieve the descriptor, select a provider, and send inference directly to its API.
The advertiser never proxies inference. A remote endpoint requires an advertiser
on each discovery LAN. No accounts, invitations, approvals, or client API keys are
required. Discovery conveys availability and claims, not identity or trust.

## Enumeration and resolution

Use DNS-SD over mDNS in `local.` following [RFC 6763](https://www.rfc-editor.org/rfc/rfc6763)
and [RFC 6762](https://www.rfc-editor.org/rfc/rfc6762). Enumerate PTR records for
`_inference._tcp.local.`, then resolve each instance's SRV, TXT, and target A/AAAA
records. Do not enumerate unrelated service types. The SRV port is the descriptor
server's port, which need not equal the inference API port. Instance names are
1–63 UTF-8 bytes. SRV ports are 1–65535. Use standard DNS-SD name escaping and
collision handling. A display name is not a persistent provider identity.

TXT contains three mandatory `key=value` strings:

| Key | v1 value | Meaning |
| --- | --- | --- |
| `v` | `1` | Discovery/descriptor major version |
| `transport` | `http` or `https` | Descriptor retrieval scheme |
| `path` | e.g. `/.well-known/inference.json` | Absolute descriptor path |

TXT MUST be at most 512 wire bytes, counting each one-byte string length;
each string is at most 255 bytes. Keys are ASCII and case insensitive; duplicates
are invalid. Unknown keys with values are ignored. Credentials MUST NOT occur in
TXT. Missing keys, unsupported versions/transports, and malformed records are
incompatible; clients MUST NOT guess defaults or downgrade versions.

`path` is 1–128 ASCII bytes of an already percent-encoded URL path beginning with
one `/`. It MUST NOT contain an authority, query, fragment, backslash, control
characters, decoded dot segments, or invalid percent escapes. Construct a URL
using the TXT scheme, SRV host and port, and that path, without interpreting it as
a relative URL. HTTP clients MAY use a resolved address in place of the host.
IPv6 literals require brackets; link-local literals require the receiving
interface's zone, percent-encoded as `%25` in URLs. HTTPS clients SHOULD use the
SRV host to preserve certificate name validation, or validate a literal against
an IP subject alternative name. Never disable certificate validation.

Services on separate interfaces are separate discovery observations. Clients
SHOULD expose interface selection and MUST NOT infer an endpoint by probing
unrelated services. Goodbyes, TTL expiration, and changed records invalidate
cached resolution. Abruptly lost providers may remain visible until expiry or a
fresh discovery pass. A snapshot UI MUST describe its freshness limits. Fetch a
fresh descriptor when connecting. Metadata changes do not migrate an active chat.

An explicit descriptor URL MUST provide a multicast-free connection path with
the same validation, authentication, and inference behavior. There is no HTTP
inference retry or failover implied by trying alternate descriptor addresses.

## Descriptor retrieval and representation

GET the descriptor with no credentials. A successful response is HTTP 200 with
`Content-Type: application/json` (optional parameters allowed), UTF-8 JSON, and
at most 32,768 body bytes. Clients MUST bound headers, body size and elapsed time.
Redirects MUST NOT be followed. No cookies, ambient authorization, proxy
credentials, `.netrc`, cloud credentials, or SDK environment keys may be used.
The reference client disables environment proxies completely.

Descriptors are objects. Required fields appear below; `default_model` is optional:

```json
{
  "version": 1,
  "name": "Office AI",
  "api": {
    "base_url": "http://192.168.1.20:8000/v1",
    "profiles": ["openai-chat-completions"],
    "capabilities": ["streaming"],
    "default_model": "office-chat",
    "models": [
      {"id": "office-chat"},
      {"id": "office-reasoner", "reasoning": true,
       "reasoning_efforts": ["low", "medium", "high"],
       "context_window": 131072, "max_output_tokens": 16384}
    ]
  },
  "auth": { "methods": ["none"] }
}
```

| Field | Semantics |
| --- | --- |
| `version` | Integer major version, exactly 1 for this contract. Unknown versions stop resolution. |
| `name` | Human-readable label, 1–128 UTF-8 bytes; independent of a collision-renamed DNS instance. |
| `api.base_url` | Absolute HTTP(S) API root, at most 2,048 UTF-8 bytes. It may name a LAN or remote server. |
| `api.profiles` | Nonempty array of supported API contracts. Client must support at least one. |
| `api.capabilities` | Required array of optional features; may be empty. Only `streaming` is defined here. |
| `api.default_model` | Optional model ID. Must match an entry in `api.models`. |
| `api.models` | Required catalog of 1–128 models with unique IDs; see below. |
| `auth.methods` | Nonempty explicit array. v0 connections require exactly `["none"]`. |

Profiles, capabilities, and authentication methods contain unique, case-sensitive identifiers matching
`[a-z][a-z0-9-]{0,63}`, with at most 32 entries. Text fields MUST NOT be blank or
contain Unicode control or format characters. Required fields cannot be null.
Duplicate JSON member names are invalid, even in unknown extensions. Maximum
JSON nesting depth is 16 (root depth 0). Trailing JSON values are invalid.
Field names are case sensitive. Unknown members are ignored, do not override
known members, and confer no behavior. Extensions SHOULD use a vendor prefix
such as `x-example`. Size and nesting limits include unknown members.

Unknown profiles and capabilities may coexist with known ones and are ignored
unless required for the requested operation. Claims are unverified assertions.
Unknown authentication methods are handled more strictly: the v0 resolver
rejects any list other than exactly `["none"]`, including mixed method lists.
Future major versions may introduce new required structure. Additive optional
members may use version 1 if ignoring them preserves all existing semantics.

## Model catalog

Each entry in `api.models` describes an available text model at `api.base_url`.
All listed models MUST support the advertised profile and capabilities.

| Field | Semantics |
| --- | --- |
| `id` | Required exact API model ID, 1–256 UTF-8 bytes; unique within the catalog. |
| `name` | Optional display name, 1–128 UTF-8 bytes. |
| `reasoning` | Optional boolean: whether the model supports reasoning. Omitted means unknown. |
| `reasoning_efforts` | Optional nonempty list of supported effort identifiers; requires `reasoning: true`. |
| `context_window` | Optional maximum combined input/output context, in tokens. |
| `max_output_tokens` | Optional maximum output length, in tokens; includes reasoning tokens when the endpoint counts them. |

Efforts are unique identifiers matching `[a-z][a-z0-9-]{0,63}`, at most 32 entries.
Common values are `low`, `medium`, and `high`. They describe model options;
they do not imply a universal request parameter or ordering. A client MUST NOT
guess a provider-specific effort mapping. The reference chat client uses the
endpoint's default effort. Reasoning support alone does not imply adjustable effort.

Token limits are integers from 1 to 2,147,483,647. When both are supplied,
`max_output_tokens` MUST NOT exceed `context_window`. Optional fields MUST NOT be
null. Omitted properties are unknown; clients MUST NOT infer limits or reasoning
support from a model's name. Model properties are operator claims, not attestations.
Pricing and token costs are not part of the catalog. Advertisers MUST NOT copy
upstream pricing or arbitrary upstream metadata into the descriptor.

Clients select models from this catalog. They need not query the inference API
for model information during connection. A fresh descriptor may add, remove,
or update models; an active conversation keeps its selected model and is never
replayed against a replacement.

## URL and API profile rules

URLs MUST have an explicit `http` or `https` scheme and a valid host, with a valid
port if present. Reject embedded userinfo, query strings (including an empty `?`),
fragments, opaque URLs, backslashes, controls, and decoded `.` or `..` path
segments. DNS hosts use ASCII labels (international names use punycode). HTTPS
uses ordinary platform root certificates and host validation.

For the `openai-chat-completions` profile, append `models` and `chat/completions`
to the API base after trimming trailing slashes. Preserve any existing prefix.
For example, `https://host/service/v1/` yields
`https://host/service/v1/models`, never `https://host/models`.

`GET <base>/models` returns HTTP 200 JSON with a nonempty `data` array of objects
containing string `id` fields. The reference accepts 1–4096 entries and a body
up to 1 MiB. The advertiser checks this endpoint before announcing and
periodically thereafter. Model availability is distinct from actual completion
readiness; `/models` alone cannot prove that generation or streaming works.

Clients choose a user override if given, otherwise the advertised default,
otherwise the catalog model with the lexicographically smallest ID (UTF-8 byte
order). A missing explicit default or override fails; do not silently substitute
another model. v0 does not infer model modality from names. The reference
advertiser can filter a mixed upstream catalog using its configured model list.

`POST <base>/chat/completions` uses JSON `model`, `messages` (text `role` and
`content`), and boolean `stream`. The initial profile covers user/assistant text
conversations. Non-streaming responses contain `choices[0].message.content` as a
string. Tool calls, image/audio inputs, Responses, embeddings, and structured
outputs are outside this profile. Future profiles get separate identifiers.

Streaming requires the explicit `streaming` capability. The response is HTTP
200 `text/event-stream`. Parse SSE lines and blank-line event boundaries,
including CRLF, comments, and joined multiline `data:` fields. JSON chunks carry
`choices[].index` and `delta.content`; role-only chunks, finish chunks, and usage
chunks are permitted. Concatenate text from choice index 0. Terminate only upon a
complete `data: [DONE]` event. EOF without it is an interrupted completion.
Malformed events, upstream error objects, unsupported tool calls, timeout or
disconnect stop the conversation. Never replay an inference POST automatically.
The reference bounds each SSE line/event to 64 KiB, wire stream to 8 MiB, generated
text to 1 MiB, request body to 1 MiB and conversation to 128 messages.

## Connection and authentication extension point

1. Resolve provider and descriptor location.
2. Fetch and validate the descriptor.
3. Apply user selection; display the API endpoint and encrypted/unencrypted state.
4. Resolve the explicit authentication method for the selected endpoint.
5. Select a compatible model from `api.models`.
6. Create the inference client and begin the conversation.

The Go `authentication.Resolver` yields a scoped `Session` authorizer. Only `none`
is implemented, and it supplies no credentials. Future invitation or approval
methods belong at step 4, before model access. Unsupported methods stop the
connection. Authentication failures MUST NOT fall back to anonymous access.
Future credentials MUST be bound to the intended scheme, host, port and API path
scope. Redirects and changed descriptors cannot extend that scope. Enrollment,
credential storage, accounts, and credential-protecting proxies are deferred.

## Errors and lifecycle

Expose actionable categories: discovery unavailable/no providers, invalid
advertisement, invalid descriptor, unsupported version/profile/capability/auth,
descriptor unreachable, model unavailable, HTTP error, TLS error, timeout, and
interrupted stream. Do not print arbitrary HTTP error bodies or credentials.
HTTP 401/403 means authentication failure, not an invitation to retry anonymously.
The CLI exits nonzero on a failed connection/inspection; a successful empty
discovery snapshot is not a transport failure. Discovery JSON retains unavailable
records with an `error` field. Watch events use `added`, `updated`, `removed`.

Advertisers withdraw on failed health checks and send goodbyes on clean shutdown.
While unhealthy, their descriptor URL returns 503. Recovery may reannounce;
clients make a new explicit connection. Descriptor changes may update model
metadata while keeping the same TXT location. A withdrawn advertisement does not
revoke endpoint access. Crash disappearance is best effort and bounded by
discovery cache behavior, not an access control mechanism.

See [security](security.md), the [schema](schemas/descriptor.schema.json), and
[conformance fixtures](../conformance/README.md). The schema covers structure;
protocol byte limits, URL semantics, duplicates and connection behavior require
additional validation.

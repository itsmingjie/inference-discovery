# Inference Discovery

Find and use open inference endpoints on a local network.

Inference Discovery is a small DNS-SD protocol with a Go reference CLI. An
advertiser publishes a JSON descriptor for an existing API; clients send
inference requests directly to that API. The descriptor includes available models
and optional reasoning and context limits, without pricing information.

```sh
# On the provider's machine
inference advertise --name "Office AI" --endpoint http://192.168.1.20:8000/v1

# On another machine on the LAN
inference chat
```

No accounts or client API keys are required. If multiple network interfaces are
available, select one with `--interface`.

## Build

Requires Go 1.24 or newer.

```sh
git clone https://github.com/itsmingjie/inference-discovery.git
cd inference-discovery
make build
./bin/inference --help
```

## Commands

| Command | Purpose |
| --- | --- |
| `inference advertise` | Advertise and monitor an existing endpoint |
| `inference discover` | List providers; supports `--json` and `--watch` |
| `inference inspect` | Show a descriptor and its compatibility |
| `inference chat` | Select a provider and start a text conversation |

When multicast is unavailable, pass an explicit descriptor URL:

```sh
inference chat --descriptor-url http://192.168.1.20:8081/.well-known/inference.json
```

Follow the [quickstart](docs/quickstart.md) for a two-machine walkthrough, or run
`make demo` for a deterministic mock endpoint. [Deployment](docs/deployment.md)
covers configuration, interfaces, HTTPS, and remote APIs. See [model catalogs](docs/models.md)
to share several models with different properties.

## Security

Open access permits anyone who can reach the endpoint to use it. Discovery does
not establish identity or guarantee that processing stays local. The CLI shows
the destination and transport encryption before connecting.

HTTP is supported for LAN demos. HTTPS uses normal certificate validation.
Clients do not forward ambient credentials, follow redirects, or replay failed
inference requests. Authenticated upstreams require an operator-managed gateway;
this repository does not implement one. Read the [security model](spec/security.md).

## For implementors

- [Protocol specification](spec/protocol.md) and [JSON Schema](spec/schemas/descriptor.schema.json)
- [Conformance cases](conformance/README.md)
- [Contributing](CONTRIBUTING.md) and [release process](docs/releasing.md)

The specification and fixtures are language independent. The reference code lives
under `reference/go`. Authentication enrollment, client integrations, and public
SDKs are deferred.

[MIT license](LICENSE).

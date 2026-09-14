# Testing and compatibility

## Automated checks

```sh
make test
python3 -m venv .venv
.venv/bin/pip install -r conformance/requirements.txt
make schema-check PYTHON=.venv/bin/python
```

Tests cover descriptor policy, authentication rejection, endpoint scope,
credential isolation, redirects, TLS validation, model selection, interrupted
streams, and the CLI's descriptor-URL flow. Standard JSON/SSE behavior is left to
the libraries; regression cases cover the additional protocol requirements.
Advertiser tests also cover delayed endpoint startup, withdrawal and recovery,
capability-check cooldowns, and model removal.

For the [Pi extension](../integrations/pi/README.md), run `make build`, then
`npm ci`, `npm run check`, and `npm test` from `integrations/pi`. Its tests use
the real CLI and Pi runtime with local HTTP servers, including package loading
without the CLI on PATH; no account or multicast is required.

For a parser fuzz campaign:

```sh
cd reference/go
go test ./internal/descriptor -fuzz=FuzzParse -fuzztime=30s
go test ./internal/inference -fuzz=FuzzStream -fuzztime=30s
```

## Network tests

Network tests are opt-in and never run in ordinary CI:

```sh
make interop INTERFACE=en0
```

This uses a local mock API and checks discovery, duplicate providers, streaming,
health withdrawal, HTTP 503, recovery, and removal. It is a same-machine test.
Follow [network interoperability](network-interop.md) for Bonjour, Avahi, and
cross-machine checks.

## Known limitations

The browser compares fresh snapshots. Fragmented DNS answers and packet loss
can produce incomplete records or temporary disappearance. Registration does not
monitor macOS interface changes; restart after changing networks. The registrar
may open multicast sockets beyond the selected announcement interface.

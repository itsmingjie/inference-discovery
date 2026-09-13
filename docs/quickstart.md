# Quickstart

Use two machines on the same LAN for the acceptance walkthrough. These commands
are examples; replace the provider's LAN address and interface with your own.
Go 1.24+ is needed only when building from source. No client API key is used.

## Build

```sh
cd reference/go
go build -o ../../bin/inference ./cmd/inference
cd ../..
export PATH="$PWD/bin:$PATH"
```

## Provider machine

Start an existing server that provides text `/v1/models` and
`/v1/chat/completions`, listening on a LAN-reachable address. For a deterministic
mock, use `go run examples/local-provider/mock.go --listen :8000` in another
terminal; this does not validate a real model.

```sh
inference advertise \
  --name "Office AI" \
  --endpoint http://192.168.1.20:8000/v1 \
  --interface en0 \
  --listen :8081
```

The minimal command omits `--interface` when exactly one eligible interface
exists. The default metadata port is ephemeral; `--listen :8081` makes the
fallback URL easy to share. Metadata binds all local addresses by default; use
a firewall to restrict access. Inference traffic goes straight to the API.

If the server lists multiple modalities, use a [configured model list](models.md)
to publish only text models. `--model MODEL` sets the default; it does not filter
the catalog. `--no-stream` withdraws the streaming claim for servers that do
not implement SSE. A `/models` preflight is not a completion test.

## Client machine

```sh
inference discover --interface en0
inference inspect --interface en0 --provider "Office AI"
inference chat --interface en0
```

The menu displays the endpoint and HTTP/HTTPS state before inference. Select
the provider, type a prompt, and watch text stream. A single provider is selected
automatically after its endpoint is displayed. Use `/quit` to finish. If you
receive a profile, auth, model, or TLS error, fix the provider configuration;
the client does not downgrade authentication or switch services.

```sh
inference discover --interface en0 --json
inference discover --interface en0 --watch --json
inference chat --interface en0 --provider "Office AI" --model MODEL
```

## Multicast-free fallback

```sh
inference inspect --descriptor-url http://192.168.1.20:8081/.well-known/inference.json
inference chat --descriptor-url http://192.168.1.20:8081/.well-known/inference.json
```

This bypasses discovery only. Descriptor validation, endpoint display,
authentication rejection, model selection, and inference behavior remain the same.

## Acceptance exercise

Run a second advertiser named `Research GPU`, then verify both are selectable.
Stop one advertiser and confirm a fresh discovery snapshot/watch removes it.
Stop the API while leaving its advertiser running: within the health interval
plus request timeout it withdraws and metadata returns 503. Restart the API and
confirm it reappears. An active interrupted chat stops and does not move.

For actual Bonjour/Avahi and IPv6 checks, follow
[network interoperability](network-interop.md) for cross-machine validation.

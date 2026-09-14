# Quickstart

Use two machines on the same LAN for the acceptance walkthrough. These commands
are examples; replace the provider's LAN address with your own.
Go 1.24+ is needed only when building from source. No client API key is used.

## Build

```sh
make build
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
  --endpoint http://192.168.1.20:8000/v1
```

The interface is selected automatically. If the OS cannot resolve the choice,
the CLI lists candidates for `--interface`. Metadata uses an ephemeral port and
binds all local addresses by default; use a firewall to restrict access.
Inference traffic goes straight to the API.

The advertiser reads `/models` and checks streaming and function calling for each
model with short synthetic prompts. It detects Chat Completions and Responses
automatically. Models with inconclusive checks are skipped with an explanation;
the deterministic mock above is correctly detected as text-only.

Use a [configured model list](models.md) to limit which models are checked and
published. `--model MODEL` sets the default; it does not filter the catalog.
Unusual or non-streaming servers can use the manual overrides described there.

## Client machine

```sh
inference chat
```

The menu displays the endpoint and HTTP/HTTPS state before inference. Select
the provider and type a prompt. Chat streams when the selected model supports it;
otherwise it displays the completed response. A single provider is selected
automatically after its endpoint is displayed. Use `/quit` to finish. If you
receive a profile, auth, model, or TLS error, fix the provider configuration;
the client does not downgrade authentication or switch services.

```sh
inference discover --json
inference inspect --provider "Office AI"
inference discover --watch --json
inference chat --provider "Office AI" --model MODEL
```

## Multicast-free fallback

On the provider, add `--listen :8081` for a stable descriptor port. Then connect
from the client with its explicit URL:

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
plus the availability request timeout it withdraws and metadata returns 503.
New-model detection can add up to the detection timeout. Restart the API and
confirm it reappears. An active interrupted chat stops and does not move.

For actual Bonjour/Avahi and IPv6 checks, follow
[network interoperability](network-interop.md) for cross-machine validation.

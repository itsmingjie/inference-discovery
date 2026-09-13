# Local provider

Use an existing OpenAI-compatible text server on a LAN address, then run:

```sh
inference advertise --name "Office AI" --endpoint http://192.168.1.20:8000/v1 --interface en0
```

For a deterministic protocol demonstration from the repository root:

```sh
go run examples/local-provider/mock.go --listen :8000
```

In another terminal, advertise that machine's LAN address as above. On a peer:

```sh
inference chat --interface en0
```

The mock streams canned text and has one model, `demo-chat`. It proves wiring,
not real inference performance or server compatibility. Use a [configured catalog](../../docs/models.md) when your real server lists
models with different modalities. See the
[two-machine walkthrough](../../docs/quickstart.md).

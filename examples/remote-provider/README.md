# Remote provider

A remote endpoint still needs an advertiser on the clients' local LAN:

```sh
inference advertise --name "Research GPU" \
  --endpoint https://your-open-inference.example/v1 --interface en0
```

Replace the example domain with an existing reachable open Chat Completions API.
Clients discover the local metadata server and send prompts directly to the
remote HTTPS API. Discovery on a LAN does not mean inference stays on that LAN.

An authenticated upstream cannot be used directly by a `none` client. For
OpenRouter or another keyed API, first provide your own operator-managed gateway
with an open client-facing API and credentials scoped to that upstream. This
repository does not implement that gateway. Never advertise a URL containing an
API key or put one in the descriptor. A temporary loopback test gateway used
during validation is not a deployable part of the reference CLI.

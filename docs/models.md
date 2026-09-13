# Sharing multiple models

The descriptor includes a model catalog. By default, the advertiser publishes
the IDs returned by the endpoint's `/models` route. It does not guess capabilities
from model names or copy vendor metadata, including pricing.

For a mixed endpoint, or to describe model properties, supply a `models` list in
the advertiser configuration:

```json
{
  "name": "Office AI",
  "endpoint": "http://192.168.1.20:8000/v1",
  "model": "office-chat",
  "models": [
    {"id": "office-chat", "name": "Office Chat", "context_window": 32768},
    {
      "id": "office-reasoner",
      "name": "Office Reasoner",
      "reasoning": true,
      "reasoning_efforts": ["low", "medium", "high"],
      "context_window": 131072,
      "max_output_tokens": 16384
    }
  ]
}
```

Replace these example IDs and limits with values supported by your server, then
run `inference advertise --config office.json`. Only configured models currently
listed by the endpoint are published. Every published model must support text
Chat Completions and the provider's streaming setting. A configured list does
not restrict direct endpoint access; it controls discovery metadata.

Only `id` is required for each model. Omit unknown properties. `reasoning: false`
means no reasoning support; omission means unknown. Effort levels describe available
options, but the reference CLI uses the server's default effort. Token limits
describe capacity, not cost. No pricing fields are published.

The catalog is refreshed on each health check. If no configured models remain,
or an explicit default is unavailable, the advertiser withdraws. Without an
explicit default, the first available ID in sorted order becomes the default.
Config edits require a restart. Catalogs are limited to 128 models and the entire
descriptor to 32 KiB; use a smaller configured list if the endpoint exceeds these.

Inspect a provider to see all model properties, or select a model by its ID:

```sh
inference inspect --provider "Office AI"
inference chat --provider "Office AI" --model office-reasoner
```

Without `--model`, chat uses the advertised default. Model selection uses the
descriptor received during connection and does not require another `/models`
request. A disappearing model stops the conversation on the next failed request;
the client never substitutes a model or replays the request.

See the [model contract](../spec/protocol.md#model-catalog) for field limits.

# Sharing multiple models

The advertiser reads `/models`, detects the APIs each model supports, and
publishes a catalog. Supply the endpoint; clients choose the API automatically.
Streaming text and function calls are checked separately. A server accepting a
`tools` parameter is insufficient: the check must produce a valid function call.

Checks use synthetic prompts, at most four requests per model, with output limits
of 128 tokens. They never execute returned tools. Four models can be checked at
once; `--timeout` bounds each request (default 10s), and `--detection-timeout`
bounds the batch (default 2m). A check reads at most 64 KiB, with 16 KiB SSE
events. Normal certificate validation and the credential/redirect restrictions
apply to all checks.

Successful checks are cached while a model remains listed. Health polls refresh
availability without repeating successful generation checks; inconclusive checks
retry at most once a minute. A removed model is forgotten and checked again if it
returns. Restart to recheck a continuously listed ID after changing its API support.
These checks are representative tests, not a guarantee that every request shape
works. Inconclusive models are skipped with a diagnostic. If no usable models
remain, the advertiser withdraws and keeps checking for recovery.

Known properties from `/models` are retained: `name`, `reasoning`,
`reasoning_efforts`, `context_window` and `max_output_tokens`. OpenRouter-style
`context_length`, `top_provider.max_completion_tokens`, and reasoning entries in
`supported_parameters` are also recognized. Explicit per-model `api` metadata
uses the contract below and bypasses generation checks for that model. Arbitrary
vendor fields and pricing are discarded; capabilities are never guessed from names.

Most servers need no model configuration. To supply missing properties or limit
the advertised catalog, add a `models` list to the advertiser configuration:

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
listed by the endpoint are considered. Configuration supplies the properties for
those models; missing API information is detected. A configured list does
not restrict direct endpoint access; it controls discovery metadata.

Only `id` is required for each model. Omit unknown properties. `reasoning: false`
means no reasoning support; omission means unknown. Effort levels describe available
options, but `inference chat` uses the server's default effort. Token limits
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

Without `--model`, chat uses the advertised default when compatible, otherwise
the first compatible model in sorted order. Model selection uses the
descriptor received during connection and does not require another `/models`
request. A disappearing model stops the conversation on the next failed request;
the client never substitutes a model or replays the request.

Chat automatically streams when the selected model advertises `streaming` and
uses a normal JSON response otherwise. It does not retry in another mode if a
request fails. Pi requires streaming and omits non-streaming models.

See the [model contract](../spec/protocol.md#model-catalog) for field limits.

## Manual overrides

To skip generation checks, supply each model's `api` object in the configuration
or in the endpoint's `/models` response:

```json
{
  "id": "office-chat",
  "api": {
    "profiles": ["openai-chat-completions", "openai-responses"],
    "capabilities": ["streaming", "function-tools"]
  }
}
```

Verify these claims for each model. For a non-streaming server, omit `streaming`
from its capabilities; an empty capabilities array is valid. Remove the `api`
object to restore automatic detection.

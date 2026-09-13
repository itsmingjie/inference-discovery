# Conformance

[fixtures/descriptors.json](fixtures/descriptors.json) contains a small set of
language-independent cases for the protocol boundary:

- An open streaming provider with multiple models and a provider without streaming.
- Unknown optional members.
- Unsupported authentication and API profiles.
- Credential-bearing endpoint URLs, missing authentication, and duplicate keys.

Each case has a `descriptor` object or `raw` wire payload, expected descriptor
validity, and expected streaming-connection compatibility. A valid descriptor is
not necessarily usable by a v0 client.

The Go connection tests consume the same cases:

```sh
make test
```

Check the object cases against the published JSON Schema:

```sh
pip install -r conformance/requirements.txt
make schema-check
```

JSON Schema does not express raw duplicate keys, UTF-8 byte limits, all URL
policy, unique model IDs, cross-field token limits, default-model membership,
or authentication behavior. Those checks belong to the implementation;
their requirements are in [the protocol](../spec/protocol.md). General-purpose
JSON syntax and SSE framing tests belong to their respective libraries.

# Contributing

Changes to the wire contract belong in `spec/` first. Add a conformance case when
it demonstrates protocol behavior that another implementation must reproduce.

Use the standard library or an established dependency for general-purpose code.
Keep custom code focused on discovery policy, compatibility, authentication
boundaries, and user interaction. Do not add wrappers that merely rename library
functions or tests that duplicate library test suites.

```sh
make fmt
make test
make schema-check
```

The schema check requires `pip install -r conformance/requirements.txt`.
[Testing](docs/testing.md) describes fuzz and opt-in network checks. Ordinary CI
must not depend on multicast, external models, or API keys.

Keep the discovery backend replaceable and authentication explicit. Enrollment,
proxies, public SDKs, and client integrations need a concrete use case before
expanding the reference implementation.

See [releasing](docs/releasing.md) for packaging and publication, and
[SECURITY.md](SECURITY.md) for private vulnerability reports.

# Releasing

Build and test with Go 1.24+ and GoReleaser 2.

```sh
make test
make schema-check
make release-check
make snapshot
```

Snapshot archives are written to `dist/`. Each includes the binary, protocol
and usage documentation, the Pi extension and its installer, dependency license
files, and the Go runtime license. The Pi installer fetches locked npm dependencies;
it does not require Go when run from a release archive.
GoReleaser produces `SHA256SUMS` for macOS and Linux on amd64 and arm64.
The ignored `licenses/` directory is generated during packaging using `go mod vendor`.

Release builds use `-trimpath` and disable VCS stamping. Use the same source,
Go version, and GoReleaser version when comparing builds.

## Publish a prerelease

1. Run the automated tests and the [interoperability checks](network-interop.md)
   for each platform you intend to list as tested.
2. Record tested platforms and interoperability results in the release notes.
3. Tag the tested commit with a prerelease tag such as `v0.1.0-alpha.1` and push it.
4. The release workflow creates a **draft prerelease** with archives and checksums.
   Review the contents and publish it from GitHub Releases.

The manual workflow builds downloadable snapshot artifacts without publishing.
Cross-compilation alone does not qualify a platform as tested.

The Go implementation is a nested module. Binary releases use repository tags;
if publishing versioned Go module downloads later, also use Go's required
`reference/go/vX.Y.Z` module tags.

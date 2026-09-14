GO ?= go
PYTHON ?= python3
GORELEASER ?= goreleaser
NPM ?= npm
PI ?= pi

.PHONY: build install-pi test fmt schema-check demo interop release-check snapshot

build:
	mkdir -p bin
	cd reference/go && $(GO) build -o ../../bin/inference ./cmd/inference

# Release archives already contain ./inference; source checkouts build it here.
install-pi:
	@if [ ! -x ./inference ]; then $(MAKE) build; fi
	$(NPM) ci --prefix integrations/pi --omit=dev --legacy-peer-deps --ignore-scripts
	$(PI) install ./integrations/pi

test:
	cd reference/go && $(GO) test -race ./... && $(GO) vet ./...

fmt:
	cd reference/go && $(GO) fmt ./...
	$(GO) fmt examples/local-provider/mock.go

schema-check:
	$(PYTHON) conformance/check_schema.py

demo:
	$(GO) run examples/local-provider/mock.go --listen :8000

# Explicitly opt in to multicast with `make interop INTERFACE=en0`.
interop: build
	@test -n "$(INTERFACE)" || (echo 'Set INTERFACE to the multicast interface' >&2; exit 2)
	$(GO) build -o bin/mock examples/local-provider/mock.go
	$(PYTHON) scripts/network-smoke.py --interface "$(INTERFACE)"

release-check:
	$(GORELEASER) check

snapshot:
	$(GORELEASER) release --snapshot --clean

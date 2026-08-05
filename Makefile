.PHONY: build install test clean dist

GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# Release string baked into the binary. Defaults to the current git
# description with any leading "v" stripped, so tag v0.1.0 reports 0.1.0.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(VERSION),)
VERSION := dev
endif

VERSION_LDFLAGS := -X github.com/TJC-LP/quartr-cli/internal/quartr.buildVersion=$(VERSION)
RELEASE_LDFLAGS := -s -w $(VERSION_LDFLAGS)

# os/arch pairs published on every release.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

build:
	go build -ldflags "$(VERSION_LDFLAGS)" -o bin/quartr ./cmd/quartr

install:
	go install -ldflags "$(VERSION_LDFLAGS)" ./cmd/quartr
	@if [ -n "$$QUARTR_API_KEY" ]; then printf '%s' "$$QUARTR_API_KEY" | $(GOBIN)/quartr auth login --api-key-stdin; fi

test:
	go test ./...

# dist is what the release workflow runs, so a local `make dist` and the
# published artifacts come from the same recipe. Raw binaries land in
# dist/bin for smoke tests; the archives and checksum file get uploaded.
dist:
	rm -rf dist
	mkdir -p dist/bin
	@set -e; for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "==> quartr $(VERSION) $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" \
			-o dist/bin/quartr-$$os-$$arch$$ext ./cmd/quartr; \
		stage=dist/stage/quartr_$(VERSION)_$${os}_$${arch}; \
		mkdir -p $$stage; \
		cp dist/bin/quartr-$$os-$$arch$$ext $$stage/quartr$$ext; \
		cp README.md LICENSE $$stage/; \
		if [ "$$os" = "windows" ]; then \
			(cd dist/stage && zip -qr ../quartr_$(VERSION)_$${os}_$${arch}.zip quartr_$(VERSION)_$${os}_$${arch}); \
		else \
			tar -czf dist/quartr_$(VERSION)_$${os}_$${arch}.tar.gz -C dist/stage quartr_$(VERSION)_$${os}_$${arch}; \
		fi; \
	done
	@rm -rf dist/stage
	@cd dist && if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum quartr_$(VERSION)_*.tar.gz quartr_$(VERSION)_*.zip > quartr_$(VERSION)_SHA256SUMS; \
	else \
		shasum -a 256 quartr_$(VERSION)_*.tar.gz quartr_$(VERSION)_*.zip > quartr_$(VERSION)_SHA256SUMS; \
	fi
	@echo "artifacts:"; ls -1 dist/*.tar.gz dist/*.zip dist/*SHA256SUMS

clean:
	rm -rf bin dist

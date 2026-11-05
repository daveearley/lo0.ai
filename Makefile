GO        ?= go
TOOLS     := $(GO) tool -modfile=tools/go.mod
BIN       := bin/lo0
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X lo0.ai/lo0/internal/buildinfo.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
GO_MIN    := 1.27

.PHONY: toolchain build test race vet fmt lint check cross dist e2e test-install clean

# Older Go releases cannot parse go.mod at all and fail with a confusing
# "usage: go 1.23" error, so check the version before anything else runs.
toolchain:
	@$(GO) version | awk -v min=$(GO_MIN) '{ \
		v = $$3; sub(/^go/, "", v); split(v, have, "."); split(min, need, "."); \
		if (have[1] < need[1] || (have[1] == need[1] && have[2] < need[2])) { \
			printf "lo0 needs Go %s or newer, but %s reports %s.\n", min, "$(GO)", $$3 > "/dev/stderr"; \
			print "Check GOROOT and PATH, or run: make GO=/path/to/go1.27/bin/go" > "/dev/stderr"; \
			exit 1 } }'

build: toolchain
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/lo0

test: toolchain
	$(GO) test ./...

race: toolchain
	$(GO) test -race ./...

vet: toolchain
	$(GO) vet ./...

fmt: toolchain
	$(TOOLS) golangci-lint fmt ./...

lint: toolchain
	$(TOOLS) staticcheck ./...
	$(TOOLS) golangci-lint run ./...

check: vet lint race

cross: toolchain
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/lo0-$$os-$$arch$$ext ./cmd/lo0 || exit 1; \
	done

# dist is what a release publishes: every binary and checksums.txt, which
# install.sh and install.ps1 look the binaries up in.
dist: cross
	cd dist && if command -v sha256sum >/dev/null; then sha256sum lo0-*; else shasum -a 256 lo0-*; fi > checksums.txt

# e2e runs the end-to-end tests alone. LO0_E2E_BINARY=path tests a binary
# built elsewhere instead of building one.
e2e: toolchain
	$(GO) test -count=1 ./e2e/

# test-install runs the real install.sh against dist/ instead of GitHub.
test-install: dist
	sh site/test/install.sh dist $(VERSION)

clean:
	$(GO) clean
	rm -rf bin dist

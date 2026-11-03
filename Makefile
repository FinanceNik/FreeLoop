# Entry points for humans and CI. The Wails build itself lives in Taskfile.yml.

WAILS_VERSION := v3.0.0
GOBIN := $(shell go env GOPATH)/bin
# Exported so the wails3 child processes (Taskfile, dev mode) find wails3 too.
export PATH := $(GOBIN):$(PATH)
WAILS := $(GOBIN)/wails3
# OQ-021: macOS 14 minimum, for cgo code built outside the Taskfile (tests).
export MACOSX_DEPLOYMENT_TARGET := 14.0
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=14.0
export CGO_LDFLAGS := -mmacosx-version-min=14.0

.PHONY: help tools dev build package dmg release verify-repro verify-deps bindings test test-keychain verify-agents memcheck frontend-test lint fmt clean

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-15s %s\n", $$1, $$2}'

tools: ## Install the pinned wails3 CLI into GOPATH/bin
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

dev: ## Run the app in development mode with live reload (offers the scripted demo provider)
	FREELOOP_DEV=1 $(WAILS) dev -config ./build/config.yml

build: ## Build a production binary into bin/
	$(WAILS) build

package: ## Build bin/Freeloop.app (ad-hoc signed)
	$(WAILS) package

# The DMG holds the app and a link to /Applications, compressed (UDZO). It is
# as signed as the app inside: ad-hoc (OQ-009).
dmg: package ## Build bin/Freeloop.dmg from bin/Freeloop.app
	rm -rf bin/dmg bin/Freeloop.dmg
	mkdir -p bin/dmg
	cp -R bin/Freeloop.app bin/dmg/
	ln -s /Applications bin/dmg/Applications
	hdiutil create -volname Freeloop -srcfolder bin/dmg -ov -format UDZO -quiet bin/Freeloop.dmg
	rm -rf bin/dmg
	@echo "bin/Freeloop.dmg"

# What a release ships besides the app: its bill of materials (SEC-012), the
# tools it was built with, and checksums, of every file in the bundle too.
release: dmg ## Build bin/release: the DMG, its SBOM, build information and checksums
	rm -rf bin/release
	mkdir -p bin/release
	cp bin/Freeloop.dmg bin/release/
	go run ./cmd/freeloop-sbom -binary bin/Freeloop.app/Contents/MacOS/Freeloop \
		-version $$(sed -n 's/^ *version: "\(.*\)"/\1/p' build/config.yml) < frontend/package-lock.json > bin/release/Freeloop.sbom.cdx.json
	{ echo "commit $$(git rev-parse HEAD)"; go version; echo "node $$(node --version)"; clang --version | head -1; \
		echo "macOS SDK $$(xcrun --show-sdk-version)"; echo "wails3 $(WAILS_VERSION)"; } > bin/release/BUILDINFO.txt
	cd bin && find Freeloop.app -type f | LC_ALL=C sort | xargs shasum -a 256 > release/APP-FILES.sha256
	cd bin/release && shasum -a 256 Freeloop.dmg Freeloop.sbom.cdx.json BUILDINFO.txt APP-FILES.sha256 > SHA256SUMS
	@echo "bin/release"

verify-repro: ## Build twice from clean clones and compare every file (NFR-SEC-004; REF=<tag> for a release)
	scripts/verify-repro.sh $(REF)

verify-deps: ## Check modules against go.sum and that go.mod is tidy (needs the network)
	go mod verify
	go mod tidy -diff

test: ## Run Go engine tests (race detector on) and frontend tests
	go test -race ./internal/...
	$(MAKE) frontend-test

memcheck: package ## Measure idle memory with ten sessions against the 300 MB budget (NFR-PERF-004; opens the app)
	scripts/memcheck.sh bin/Freeloop.app/Contents/MacOS/Freeloop 300

verify-agents: ## Run the HRN-005 scenario with each preset's real agent on a local stand-in model (HRN-007; downloads the agents)
	scripts/verify-agents.sh

test-keychain: ## Run the Keychain backend test against the real login keychain
	FREELOOP_KEYCHAIN_TEST=1 go test -count=1 -run '^(TestKeychain|TestSeatbeltClosesTheKeychain)$$' ./internal/platform/darwin/

frontend-test: frontend/node_modules
	cd frontend && npm test

bindings: ## Regenerate the TypeScript bindings for the Go services
	$(WAILS) generate bindings -clean=true -ts -i -silent ./cmd/freeloop

lint: frontend/node_modules bindings ## gofmt, go vet, architecture boundaries (ENV-001) and TypeScript
	@test -z "$$(gofmt -l cmd internal frontend/embed.go)" || { gofmt -l cmd internal frontend/embed.go; echo "gofmt needed"; exit 1; }
	go vet ./internal/...
	go test -count=1 ./internal/archtest/
	cd frontend && npm run typecheck

fmt: ## Format Go code
	gofmt -w cmd internal frontend/embed.go

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci
	@touch $@

clean: ## Remove build output
	rm -rf bin frontend/dist .task

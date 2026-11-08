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

.PHONY: help tools dev build package check-app dmg notices release verify-repro verify-deps bindings test cmd-test test-keychain verify-agents memcheck frontend-test lint fmt clean

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-15s %s\n", $$1, $$2}'

tools: ## Install the pinned wails3 CLI into GOPATH/bin
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

dev: ## Run the app in development mode with live reload (offers the scripted demo provider)
	FREELOOP_DEV=1 $(WAILS) dev -config ./build/config.yml

build: ## Build a production binary into bin/
	$(WAILS) build

package: ## Build bin/Freeloop.app (ad-hoc signed; SHIPPED=1 for the build users get)
	$(WAILS) package $(if $(filter 1,$(SHIPPED)),SHIPPED=1)

# The DMG holds the app and a link to /Applications, compressed (UDZO). It is
# as signed as the app inside: ad-hoc (OQ-009). Its app is the build users get,
# which reads no development switch (FREELOOP_DEV, FREELOOP_DEV_*, FREELOOP_HOME,
# FREELOOP_RELEASE_API; cmd/freeloop/devenv.go). make passes SHIPPED on to the
# package it builds for the DMG, and so for a release; `make package` alone,
# which memcheck and local rebuilds use, keeps the switches.
dmg: SHIPPED := 1
dmg: package notices ## Build bin/Freeloop.dmg from bin/Freeloop.app
	rm -rf bin/dmg bin/Freeloop.dmg
	mkdir -p bin/dmg
	cp -R bin/Freeloop.app bin/dmg/
	cp bin/THIRD_PARTY_NOTICES.txt bin/dmg/
	ln -s /Applications bin/dmg/Applications
	hdiutil create -volname Freeloop -srcfolder bin/dmg -ov -format UDZO -quiet bin/Freeloop.dmg
	rm -rf bin/dmg
	@echo "bin/Freeloop.dmg"

# The licenses of everything in the app, which its licenses say must go with it.
notices: ## Write bin/THIRD_PARTY_NOTICES.txt from what bin/Freeloop.app was built with
	go run ./cmd/freeloop-sbom -notices -modcache "$$(go env GOMODCACHE)" -node-modules frontend/node_modules \
		-binary bin/Freeloop.app/Contents/MacOS/Freeloop \
		-version $$(sed -n 's/^ *version: "\(.*\)"/\1/p' build/config.yml) < frontend/package-lock.json > bin/THIRD_PARTY_NOTICES.txt

# What a release ships besides the app: its bill of materials (SEC-012), the
# tools it was built with, and checksums, of every file in the bundle too.
release: dmg ## Build bin/release: the DMG, its SBOM, build information and checksums
	rm -rf bin/release
	mkdir -p bin/release
	cp bin/Freeloop.dmg bin/THIRD_PARTY_NOTICES.txt bin/release/
	go run ./cmd/freeloop-sbom -binary bin/Freeloop.app/Contents/MacOS/Freeloop \
		-version $$(sed -n 's/^ *version: "\(.*\)"/\1/p' build/config.yml) < frontend/package-lock.json > bin/release/Freeloop.sbom.cdx.json
	{ echo "commit $$(git rev-parse HEAD)"; go version; echo "node $$(node --version)"; clang --version | head -1; \
		echo "macOS SDK $$(xcrun --show-sdk-version)"; echo "wails3 $(WAILS_VERSION)"; } > bin/release/BUILDINFO.txt
	cd bin && find Freeloop.app -type f | LC_ALL=C sort | xargs shasum -a 256 > release/APP-FILES.sha256
	cd bin/release && shasum -a 256 Freeloop.dmg Freeloop.sbom.cdx.json THIRD_PARTY_NOTICES.txt BUILDINFO.txt APP-FILES.sha256 > SHA256SUMS
	@echo "bin/release"

verify-repro: ## Build twice from clean clones and compare every file (NFR-SEC-004; REF=<tag> for a release)
	scripts/verify-repro.sh $(REF)

verify-deps: ## Check modules against go.sum and that go.mod is tidy (needs the network)
	go mod verify
	go mod tidy -diff

# Every event a test writes is checked against the schema (EXT-002).
test: export FREELOOP_CHECK_EVENTS := $(CURDIR)/schemas/events.json
test: ## Run Go tests (race detector on) and frontend tests
	go test -race ./internal/...
	$(MAKE) cmd-test
	$(MAKE) frontend-test

# cmd/freeloop embeds the built frontend, so it needs one to compile.
cmd-test: frontend/node_modules bindings
	cd frontend && npm run build
	go test -race ./cmd/...

memcheck: package ## Measure idle memory with ten sessions against the 300 MB budget (NFR-PERF-004; opens the app)
	scripts/memcheck.sh bin/Freeloop.app/Contents/MacOS/Freeloop 300

check-app: ## Drive the real interface in a real WebKit view, on scratch data with the demo provider
	scripts/appcheck.sh

verify-agents: ## Run the HRN-005 scenario with each preset's real agent on a local stand-in model (HRN-007; downloads the agents)
	scripts/verify-agents.sh

test-keychain: ## Run the Keychain backend test against the real login keychain
	FREELOOP_KEYCHAIN_TEST=1 go test -count=1 -run '^(TestKeychain|TestKeychainVault|TestSeatbeltClosesTheKeychain)$$' ./internal/platform/darwin/

frontend-test: frontend/node_modules
	cd frontend && npm test

# Linux (NFR-XPLAT-003) is built and tested in a container from build/linux/Dockerfile,
# so a Mac with Docker (Colima) can do both. The source is mounted read-only; module
# and build caches live in a volume between runs.
LINUX_ARCH ?= arm64
LINUX_IMAGE := freeloop-linux-$(LINUX_ARCH)
LINUX_RUN := docker run --rm --platform linux/$(LINUX_ARCH) -v "$(CURDIR)":/src:ro -v freeloop-linux-cache:/home/build/go -v freeloop-linux-buildcache:/home/build/.cache -e FREELOOP_CHECK_EVENTS=/src/schemas/events.json $(LINUX_IMAGE)

linux-image: ## Build the Linux build and test image (needs Docker)
	docker build --platform linux/$(LINUX_ARCH) -t $(LINUX_IMAGE) build/linux

linux-test: linux-image ## Run the Go tests on Linux in the build image (race detector on)
	$(LINUX_RUN) go test -race ./internal/...

# Needs no desktop: a private session bus and a GNOME Keyring that gnome-keyring-daemon
# --unlock creates (the login keyring, empty password) and opens, as a login does. The
# way to run the same on a Linux desktop is in internal/platform/linux/secretservice_test.go.
linux-test-keyring: linux-image ## Run the Secret Service tests against a real GNOME Keyring in the build image
	$(LINUX_RUN) dbus-run-session -- sh -c 'echo "" | gnome-keyring-daemon --unlock --components=secrets >/dev/null; FREELOOP_KEYRING_TEST=1 go test -race -count=1 -run "^TestSecretService" ./internal/platform/linux/'

# Compiles the app for Linux (Wails needs cgo and GTK, so this cannot be done from the Mac
# directly) and vets cmd/freeloop there. The source is copied, since it is mounted read-only.
linux-build: linux-image frontend/node_modules ## Compile and vet the Linux app in the build image
	cd frontend && npm run build
	$(LINUX_RUN) sh -c 'mkdir /tmp/app && cd /src && tar cf - --exclude=node_modules --exclude=.git --exclude=bin . | tar xf - -C /tmp/app && cd /tmp/app && go vet -tags gtk3,production ./cmd/freeloop && go build -tags gtk3,production -o /tmp/freeloop ./cmd/freeloop && file /tmp/freeloop'

# The sandbox's tests need bubblewrap and user namespaces: a privileged container,
# in a VM that allows unprivileged user namespaces (Colima's Ubuntu restricts them
# through AppArmor; `colima ssh -- sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`).
linux-test-sandbox: linux-image ## Try to break out of the Linux sandbox, in a privileged container
	docker run --rm --privileged --platform linux/$(LINUX_ARCH) -v "$(CURDIR)":/src:ro -v freeloop-linux-cache:/home/build/go -v freeloop-linux-buildcache:/home/build/.cache -e FREELOOP_SANDBOX_TEST=1 $(LINUX_IMAGE) go test -race -count=1 ./internal/platform/linux/

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

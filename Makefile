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
# The app's version, as build/config.yml has it.
VERSION := $(shell sed -n 's/^ *version: "\(.*\)"/\1/p' build/config.yml)
# shasum on macOS; sha256sum where Perl's shasum is missing (Git for Windows). Same output.
SHA256 := $(shell command -v shasum >/dev/null 2>&1 && echo "shasum -a 256" || echo sha256sum)

.PHONY: help tools dev build package whisper-cli install check-app dmg notices windows windows-exe windows-tools linux-package linux-package-arch release release-mac release-windows release-linux release-sums release-sign release-key verify-repro verify-deps bindings test cmd-test test-keychain verify-agents memcheck frontend-test lint fmt clean

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-19s %s\n", $$1, $$2}'

tools: ## Install the pinned wails3 CLI into GOPATH/bin
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

dev: ## Run the app in development mode with live reload (offers the scripted demo provider)
	FREELOOP_DEV=1 $(WAILS) dev -config ./build/config.yml

build: ## Build a production binary into bin/
	$(WAILS) build

# SIGN_IDENTITY is a signing identity in the keychain, "Developer ID Application: Name
# (TEAMID)", for a release others download; without it the app is signed ad hoc.
# NOTARY_PROFILE is a notarytool keychain profile (xcrun notarytool store-credentials),
# with which `make dmg` has Apple notarize the disk image (SEC-021; DEVELOPING.md,
# "Signing a release").
package: whisper-cli ## Build bin/Freeloop.app (ad-hoc signed, or SIGN_IDENTITY=...; SHIPPED=1 for the build users get)
	$(WAILS) package $(if $(filter 1,$(SHIPPED)),SHIPPED=1) $(if $(SIGN_IDENTITY),SIGN_IDENTITY="$(SIGN_IDENTITY)")

# The speech program voice input runs (GUI-050), which the Mac app carries; needs cmake.
whisper-cli: ## Build bin/whisper-cli from whisper.cpp's pinned source (macOS)
	scripts/whisper-cli.sh

install: ## Quit Freeloop, rebuild it from this commit and leave that as the only copy (INSTALL_DIR=~/Applications, OPEN=1)
	scripts/install.sh

# The DMG holds the app and a link to /Applications, compressed (UDZO). It is
# as signed as the app inside: ad hoc, or by SIGN_IDENTITY and then notarized
# with NOTARY_PROFILE (OQ-056). Its app is the build users get,
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
	@if [ -n "$(SIGN_IDENTITY)" ]; then codesign --force --timestamp --sign "$(SIGN_IDENTITY)" bin/Freeloop.dmg || exit 1; \
		else echo "  the app and the disk image are signed ad hoc (no SIGN_IDENTITY): macOS asks before opening a download"; fi
	@if [ -n "$(NOTARY_PROFILE)" ]; then test -n "$(SIGN_IDENTITY)" || { echo "NOTARY_PROFILE needs SIGN_IDENTITY: Apple notarizes only what a Developer ID signed"; exit 1; }; \
		xcrun notarytool submit bin/Freeloop.dmg --keychain-profile "$(NOTARY_PROFILE)" $(if $(NOTARY_KEYCHAIN),--keychain "$(NOTARY_KEYCHAIN)") --wait && xcrun stapler staple bin/Freeloop.dmg || exit 1; \
		else echo "  the disk image is not notarized (no NOTARY_PROFILE)"; fi
	@echo "bin/Freeloop.dmg"

# The speech program a Mac app carries (GUI-050), in its bill and notices.
NATIVE = $(if $(wildcard bin/Freeloop.app/Contents/MacOS/whisper-cli),-native bin/whisper-cli.json)

# The licenses of everything in the app, which its licenses say must go with it.
notices: ## Write bin/THIRD_PARTY_NOTICES.txt from what bin/Freeloop.app was built with
	go run ./cmd/freeloop-sbom -notices -modcache "$$(go env GOMODCACHE)" -node-modules frontend/node_modules \
		-binary bin/Freeloop.app/Contents/MacOS/Freeloop $(NATIVE) \
		-version $(VERSION) < frontend/package-lock.json > bin/THIRD_PARTY_NOTICES.txt

# Windows (NFR-XPLAT-003) is pure Go (WebView2 through go-webview2), so it
# cross-compiles from any host with CGO_ENABLED=0; build/windows/Taskfile.yml has
# the steps. The installer needs NSIS 3's makensis, the zip needs zip. Both are
# unsigned for now, as the Mac build is ad-hoc signed (OQ-009).
WINDOWS_ARCHS := amd64 arm64

windows-tools:
	@command -v makensis >/dev/null 2>&1 || { echo "makensis (NSIS 3) is missing; it makes the Windows installer."; \
		echo "  macOS: brew install makensis   Debian/Ubuntu: sudo apt install nsis   Windows: https://nsis.sourceforge.io"; exit 1; }
	@command -v zip >/dev/null 2>&1 || { echo "zip is missing; it makes the portable Windows zip."; exit 1; }

windows-exe: frontend/node_modules bindings ## Cross-compile bin/windows-{amd64,arm64}/Freeloop.exe, the build users get
	cd frontend && npm run build
	for arch in $(WINDOWS_ARCHS); do ARCH=$$arch $(WAILS) task windows:build:exe SHIPPED=1 || exit 1; done

windows: windows-tools windows-exe ## Build the Windows installers and portable zips (amd64, arm64) into bin/
	for arch in $(WINDOWS_ARCHS); do ARCH=$$arch $(WAILS) task windows:package:files || exit 1; done

# Linux packages build per architecture in the image from build/linux/Dockerfile
# (cgo with GTK 3 and WebKitGTK 4.1), natively: by default only the Docker
# machine's own architecture. LINUX_ARCHS="amd64 arm64" adds the other one,
# emulated through --platform. Under QEMU that is slow but works; under Rosetta
# (Colima's vz with rosetta: true, Docker Desktop's default) Go itself crashes
# with heap errors, so on Apple Silicon amd64 comes from CI's amd64 runner or a
# QEMU-only Docker. build/linux/package.sh runs inside and writes into bin/.
LINUX_ARCHS ?= $(subst aarch64,arm64,$(subst x86_64,amd64,$(shell docker info --format '{{.Architecture}}' 2>/dev/null)))

linux-package: frontend/node_modules bindings ## Build the Linux AppImage and .deb for LINUX_ARCHS (default: Docker's own) into bin/
	@test -n "$(LINUX_ARCHS)" || { echo "Docker is not running (Colima: colima start); Linux packages build in it"; exit 1; }
	cd frontend && npm run build
	for arch in $(LINUX_ARCHS); do $(MAKE) linux-package-arch LINUX_ARCH=$$arch || exit 1; done

linux-package-arch: linux-image
	mkdir -p bin
	docker run --rm --platform linux/$(LINUX_ARCH) -v "$(CURDIR)":/src:ro -v "$(CURDIR)/bin":/out \
		-v freeloop-linux-cache:/home/build/go -v freeloop-linux-buildcache:/home/build/.cache \
		$(LINUX_IMAGE) sh /src/build/linux/package.sh

# A release holds what this machine can build (RELEASE_PLATFORMS): the Mac on a
# Mac, Windows where makensis is, Linux where Docker runs. Naming them builds
# exactly those and fails on what cannot be built; `RELEASE_PLATFORMS=mac` is the
# Mac-only release of before. Each platform adds its files, its bill of materials
# (SEC-012) and the tools it was built with; SHA256SUMS covers every file.
RELEASE_PLATFORMS ?= $(strip $(if $(filter Darwin,$(shell uname -s)),mac) \
	$(if $(shell command -v makensis 2>/dev/null),windows) \
	$(if $(shell docker info >/dev/null 2>&1 && echo yes),linux))

release: ## Build bin/release for RELEASE_PLATFORMS (default: what this host can), with SBOMs, build information, checksums
	@test -n "$(RELEASE_PLATFORMS)" || { echo "nothing this machine can release: not macOS, no makensis, no Docker"; exit 1; }
	@echo "release for: $(RELEASE_PLATFORMS)"
	@for p in mac windows linux; do case " $(RELEASE_PLATFORMS) " in *" $$p "*) ;; *) echo "  leaving out $$p (RELEASE_PLATFORMS)";; esac; done
	rm -rf bin/release
	mkdir -p bin/release
	for p in $(RELEASE_PLATFORMS); do $(MAKE) release-$$p || exit 1; done
	$(MAKE) release-sums release-sign
	@echo "bin/release"

# The Mac: the DMG, its bill, the tools it was built with, and checksums of every
# file in the bundle.
release-mac: dmg
	mkdir -p bin/release
	cp bin/Freeloop.dmg bin/THIRD_PARTY_NOTICES.txt bin/release/
	go run ./cmd/freeloop-sbom -binary bin/Freeloop.app/Contents/MacOS/Freeloop $(NATIVE) \
		-version $(VERSION) < frontend/package-lock.json > bin/release/Freeloop.sbom.cdx.json
	{ echo "commit $$(git rev-parse HEAD)"; go version; echo "node $$(node --version)"; clang --version | head -1; \
		echo "macOS SDK $$(xcrun --show-sdk-version)"; echo "wails3 $(WAILS_VERSION)"; } > bin/release/BUILDINFO.txt
	cd bin && find Freeloop.app -type f | LC_ALL=C sort | xargs shasum -a 256 > release/APP-FILES.sha256

# Windows: the installer and the zip per architecture and a bill per exe; the
# notices are the same for both architectures.
release-windows: windows
	mkdir -p bin/release
	for arch in $(WINDOWS_ARCHS); do \
		cp bin/Freeloop-$(VERSION)-windows-$$arch-setup.exe bin/Freeloop-$(VERSION)-windows-$$arch-portable.zip bin/release/ && \
		go run ./cmd/freeloop-sbom -binary bin/windows-$$arch/Freeloop.exe -version $(VERSION) \
			< frontend/package-lock.json > bin/release/Freeloop-windows-$$arch.sbom.cdx.json || exit 1; \
	done
	cp bin/windows-$(firstword $(WINDOWS_ARCHS))/THIRD_PARTY_NOTICES.txt bin/release/THIRD_PARTY_NOTICES-windows.txt
	{ echo "commit $$(git rev-parse HEAD)"; go version; echo "node $$(node --version)"; \
		echo "GOOS=windows CGO_ENABLED=0 for $(WINDOWS_ARCHS)"; echo "makensis $$(makensis -VERSION)"; \
		echo "wails3 $(WAILS_VERSION)"; } > bin/release/BUILDINFO-windows.txt
	cd bin && ls $(foreach a,$(WINDOWS_ARCHS),windows-$(a)/Freeloop.exe) | LC_ALL=C sort | xargs $(SHA256) > release/WINDOWS-FILES.sha256

# Linux: the AppImage and the .deb per architecture, a bill per binary, and what
# the image built them with.
release-linux: linux-package
	mkdir -p bin/release
	for arch in $(LINUX_ARCHS); do \
		cp bin/Freeloop-$(VERSION)-linux-$$arch.AppImage bin/Freeloop-$(VERSION)-linux-$$arch.deb bin/release/ && \
		go run ./cmd/freeloop-sbom -binary bin/linux-$$arch/freeloop -version $(VERSION) \
			< frontend/package-lock.json > bin/release/Freeloop-linux-$$arch.sbom.cdx.json || exit 1; \
	done
	cp bin/linux-$(firstword $(LINUX_ARCHS))/THIRD_PARTY_NOTICES.txt bin/release/THIRD_PARTY_NOTICES-linux.txt
	for arch in $(LINUX_ARCHS); do \
		{ echo "commit $$(git rev-parse HEAD)"; echo "node $$(node --version), frontend built on the host"; \
			cat bin/linux-$$arch/BUILDINFO.txt; } > bin/release/BUILDINFO-linux-$$arch.txt || exit 1; \
	done

# Checksums of every file in bin/release, files another machine built and copied
# in included (CI's release job adds the Windows and Linux files that way).
release-sums:
	cd bin/release && rm -f SHA256SUMS SHA256SUMS.sig && find . -type f | sed 's|^\./||' | LC_ALL=C sort | xargs $(SHA256) > ../SHA256SUMS.tmp && mv ../SHA256SUMS.tmp SHA256SUMS

# The project's signature over the version and SHA256SUMS (SEC-021), which a copy of
# the app checks before it updates itself in place (SET-008). The key is
# RELEASE_SIGNING_KEY (CI's secret) or the file RELEASE_SIGNING_KEY_FILE names, by
# default the one `make release-key` writes. Without a key the release is left
# unsigned, loudly, and no copy updates to it in place; REQUIRE_SIGNATURE=1 fails
# instead, as CI's release job asks.
export RELEASE_SIGNING_KEY_FILE ?= $(HOME)/.config/freeloop-release/signing.key

release-sign: ## Sign bin/release/SHA256SUMS with the project's release key (after release-sums)
	@if [ -n "$$RELEASE_SIGNING_KEY" ] || [ -f "$$RELEASE_SIGNING_KEY_FILE" ]; then \
		go run ./cmd/freeloop-sign sign -version $(VERSION) -dir bin/release && \
		go run ./cmd/freeloop-sign verify -version $(VERSION) -dir bin/release; \
	elif [ "$(REQUIRE_SIGNATURE)" = 1 ]; then echo "no release signing key (RELEASE_SIGNING_KEY or $$RELEASE_SIGNING_KEY_FILE)"; exit 1; \
	else echo "NOT SIGNED: no release signing key, so no copy of the app will update to this release in place"; fi

release-key: ## Make the project's release signing key, once (never over one), and print its public half for internal/release
	go run ./cmd/freeloop-sign keygen -out "$$RELEASE_SIGNING_KEY_FILE"
	@echo "Add the line above to Keys in internal/release/release.go, and keep $$RELEASE_SIGNING_KEY_FILE safe: it is the only copy."

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
	docker build --platform linux/$(LINUX_ARCH) --build-arg WAILS_VERSION=$(WAILS_VERSION) -t $(LINUX_IMAGE) build/linux

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

lint: frontend/node_modules frontend/lint/node_modules bindings ## gofmt, go vet, architecture boundaries (ENV-001), TypeScript and the hooks rules
	@test -z "$$(gofmt -l cmd internal frontend/embed.go)" || { gofmt -l cmd internal frontend/embed.go; echo "gofmt needed"; exit 1; }
	go vet ./internal/...
	go test -count=1 ./internal/archtest/
	cd frontend && npm run typecheck
	cd frontend && lint/node_modules/.bin/eslint -c lint/eslint.config.js src

fmt: ## Format Go code
	gofmt -w cmd internal frontend/embed.go

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci
	@touch $@

# ESLint lives apart from the app's packages: typescript-eslint cannot load TypeScript 7 yet.
frontend/lint/node_modules: frontend/lint/package-lock.json
	cd frontend/lint && npm ci
	@touch $@

clean: ## Remove build output
	rm -rf bin frontend/dist .task

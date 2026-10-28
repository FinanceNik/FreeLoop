# Freeloop

An open-source macOS app for running AI coding agents with your own models, switchable harnesses, Agent Skills, MCP servers and custom environments. Apache-2.0.

Status: early. Milestone M0 (foundations) is built; see [docs/ROADMAP.md](docs/ROADMAP.md). The specification lives in [docs/](docs/).

## Develop

Needs macOS 14+, Go, Node with npm, and the Xcode command line tools.

```sh
make tools   # install the pinned wails3 CLI
make dev     # run the app with live reload
make test    # Go and frontend tests
make lint    # formatting, vet, architecture boundaries, TypeScript
```

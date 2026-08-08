# vigie

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

> **Status: bootstrapping.** The toolchain and `vigie version` are in place; features
> are being ported incrementally. See the roadmap for what's landing next.

A single CLI and declarative YAML DSL for testing Helm charts across progressively
higher-fidelity tiers — from fast in-process template rendering to full end-to-end
cluster tests.

## Building

Development uses a Nix flake (Go 1.26):

```sh
nix develop --command make build   # -> dist/vigie
./dist/vigie version
```

`make help` lists the available targets.

## Roadmap

Features land milestone by milestone: `lint`, then the DSL/schema foundation, then
the `test` (template) tier, `validate`, distribution, and the `test-apply` cluster
tiers. Each milestone is independently releasable.

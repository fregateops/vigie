# vigie

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

> **Status: template tier shipped.** `vigie lint`, `vigie test` (template-tier render +
> assertions), and `vigie schema` work end to end. The `validate` and `test-apply` cluster
> tiers are on the [roadmap](#roadmap).

A single CLI and declarative YAML DSL for testing Helm charts across progressively
higher-fidelity tiers — from fast in-process template rendering to full end-to-end cluster
tests. Helm is used as a **library**, never shelled out.

---

## Tiers

| Tier | Backend | What it proves | Status |
|---|---|---|---|
| `lint` | Static analysis | Chart hygiene, Chart.yaml, deprecations | ✅ shipped |
| `test` (template) | `helm template` in-process + assertions | Go template logic produces the expected YAML | ✅ shipped |
| `validate` | template + kubeconform | Rendered YAML is structurally valid Kubernetes | 🔜 planned |
| `test-apply api` | envtest (real apiserver + etcd) | The API server accepts the resources | 🔜 planned |
| `test-apply simulated` | envtest + controllers + kwok | Controllers reconcile, Pods start | 🔜 planned |
| `test-apply e2e` | k3d / kind / external cluster | Workloads run, real network and probes | 🔜 planned |

---

## Installation

Build from source with Nix (Go 1.26):

```sh
git clone https://github.com/fregateops/vigie
cd vigie
nix develop --command make build   # -> dist/vigie
./dist/vigie version
```

Or with a local Go 1.26+ toolchain:

```sh
go build -o vigie ./cmd/vigie
```

---

## Quick start

Drop test files under `tests/unit/` in your chart:

```yaml
# mychart/tests/unit/deployment_test.yaml
suite: deployment
templates:
  - templates/deployment.yaml

tests:
  - it: renders a Deployment with the configured replica count
    inputs:
      set:
        replicaCount: 3
    asserts:
      - isKind: Deployment
      - isAPIVersion: apps/v1
      - equal:
          path: spec.replicas
          value: 3

  - it: image tag is overridable
    inputs:
      set:
        image.tag: v2.0
    asserts:
      - matchRegex:
          path: spec.template.spec.containers[0].image
          pattern: ':v2\.0$'
```

Run them:

```sh
vigie test ./mychart
```

```
deployment  (9ms)
────────────────────────────────────────────────────────────
  PASS  renders a Deployment with the configured replica count  (1ms)
  PASS  image tag is overridable  (1ms)

────────────────────────────────────────────────────────────
Tests: 2 total, 2 passed (2ms total test time)
```

A complete, realistic example chart lives in
[`testdata/charts/basic`](./testdata/charts/basic) — its `tests/unit/` suite exercises the
full matcher library, `matrix`/`cases`, helper (`call:`) tests, and snapshots.

---

## Test file structure

```yaml
suite: <name>            # human-readable suite name
templates:               # limit rendering to these templates (optional)
  - templates/deployment.yaml

tests:
  - it: <description>
    skip: false          # true, or a non-empty string reason, skips the test
    inputs:              # helm rendering inputs (required nesting level)
      set:               # --set overrides (dot notation supported)
        image.tag: v2.0
      release:
        name: my-release
        namespace: my-ns
      capabilities:
        kubeVersion: "1.30"
    target:              # which rendered document to assert on (optional)
      kind: Deployment
      name: my-app
    asserts:
      - <matcher>: ...
```

Two ways to parameterize a test:

```yaml
  - it: works across replica counts
    matrix:
      replicaCount: [1, 3]
    inputs:
      set:
        replicaCount: ${{ matrix.replicaCount }}
    asserts:
      - equal: { path: spec.replicas, value: ${{ matrix.replicaCount }} }

  - it: builds the image reference per tag
    cases:
      - name: pinned
        set: { image.tag: "2.0.0" }
        want: "myrepo/app:2.0.0"
    asserts:
      - equal: { path: spec.template.spec.containers[0].image, value: ${{ case.want }} }
```

`${{ ... }}` interpolates a CEL expression against the current `matrix`/`case` bindings.

### Matchers

| Matcher | Description |
|---|---|
| `equal` / `notEqual: { path, value }` | Deep (in)equality at path |
| `greaterThan` / `lessThan` / `gte` / `lte: { path, value }` | Numeric comparison |
| `contains` / `notContains: { path, content }` | Substring or list membership |
| `startsWith` / `endsWith: { path, value }` | String prefix/suffix |
| `matchRegex` / `notMatchRegex: { path, pattern }` | RE2 regex |
| `matchTemplate: { path, pattern }` | `${VAR}`-placeholder template match |
| `exists` / `notExists: { path }` | Path existence |
| `isNull` / `isNotNull: { path }` | Null check |
| `isEmpty` / `isNotEmpty: { path }` | Empty string/list/map |
| `isKind: <string>` | `kind` field equals value |
| `isAPIVersion: <string>` | `apiVersion` field equals value |
| `isType: { path, of }` | Type check: string, int, float, bool, list, map |
| `lengthEqual: { path, value }` | Collection length equals N |
| `isSubset: { path, content }` | Object contains all keys/values from content |
| `matchSchema: { path, schema }` | Value validates against an inline JSON Schema fragment |
| `hasDocuments: <int>` | Number of rendered documents equals N |
| `failedTemplate: { errorPattern }` | Render failed, optionally matching a regex |
| `matchSnapshot: { path }` | Value matches on-disk snapshot (auto-created on first run) |
| `allOf` / `anyOf: [...]` | All / at least one nested assertion must pass |
| `expr: <CEL>` | CEL expression over `doc`/`resources`/`matrix`/`case` evaluates to true |

Any matcher accepts `not: true` to invert the result.

**Path syntax** is dotted with integer bracket indexing, e.g. `spec.template.spec.containers[0].image`.
Map keys that contain dots (such as the `app.kubernetes.io/name` label) are not addressable
via `path:` — use `expr:` instead, e.g. `doc.metadata.labels["app.kubernetes.io/name"] == "myapp"`.

### Selecting a document

When a test renders multiple documents, pin one with `target:`:

```yaml
target:
  kind: Deployment       # by kind
  name: my-app           # by metadata.name
  documentIndex: 0       # by position in render output (0-based)
  expr: 'doc.kind == "Service"'   # or a CEL predicate
```

Override per assertion with `on:` when a single test checks multiple documents:

```yaml
asserts:
  - isKind: Deployment
    on: { kind: Deployment }
  - isKind: Service
    on: { kind: Service }
```

Set `forEach: true` on a test to run every assertion against **all** documents matching `target`.

### Helper (`call:`) tests

Test a named template directly, without rendering a whole chart:

```yaml
suite: helpers
helpers:
  - templates/_helpers.tpl
tests:
  - it: builds an image reference
    call: mychart.image
    args:
      repository: myrepo/app
      tag: "1.2.3"
    outputAs: string        # string | yaml | json | bool
    asserts:
      - equal: { value: "myrepo/app:1.2.3" }
```

---

## CLI reference

```
vigie version                 print version information

vigie lint <chart>            static analysis: chart-yaml, best-practices, deprecations
  --rule-sets <a,b>           run only these rule sets (default: all)
  --disable-rules <a,b>       skip specific rule IDs (added to config)
  --kube-version <ver>        target Kubernetes API version for deprecation checks

vigie test <chart>            template tier: render + assert (tests/unit/*_test.yaml)
  --file <path>               run a single test file instead of discovering all
  --tests <dir>               discovery root (default: <chart>/tests)
  --snapshot-dir <dir>        snapshot directory (default: <chart>/tests/snapshots)

vigie schema                  print the test-file JSON Schema
```

Global flags: `-o, --output pretty|junit` · `-p, --parallelism <n>` · `-v` debug / `-vv` trace.

Exit codes: `0` pass · `1` test failure · `2` setup error · `3` user error · `4` infra error.

---

## Configuration

Place a `.vigie.yaml` at the chart root to set defaults. A fully-commented reference is in
[`examples/.vigie.yaml`](./examples/.vigie.yaml); a working example ships in
[`testdata/charts/basic/.vigie.yaml`](./testdata/charts/basic/.vigie.yaml).

```yaml
defaults:
  release:
    name: release-name
    namespace: default

lint:
  ruleSets: [chart-yaml, template-best-practices, deprecation]
  disableRules:
    - template-best-practices_missing-resource-limits

test:
  testsDir: tests/unit
```

### Lint rule sets

| Rule set | Checks |
|---|---|
| `helm-v3-lint` | Delegates to Helm v3's `helm lint` |
| `chart-yaml` | Chart.yaml structure: apiVersion, name, version, description |
| `template-best-practices` | Hardcoded namespaces, missing resource limits, … |
| `deprecation` | Removed/deprecated Kubernetes APIs (+ operator-specific sets) |

Rule IDs are namespaced as `<ruleSet>_<id>` (e.g. `template-best-practices_hardcoded-namespace`).

---

## Development

Requires [Nix](https://nixos.org/).

```sh
nix develop                                  # enter the dev shell (Go, golangci-lint, pre-commit)
nix develop --command make build             # build ./dist/vigie
nix develop --command make test              # pre-commit + go test ./...
nix develop --command make run ARGS="test ./testdata/charts/basic"
```

`make help` lists every target.

---

## Roadmap

Features land milestone by milestone; each is independently releasable.

| Milestone | Status | Scope |
|---|---|---|
| M0 | ✅ | Toolchain bootstrap, `vigie version`, release pipeline |
| M1 | ✅ | `vigie lint` — chart-yaml, best-practices, deprecation rule sets |
| M2 | ✅ | DSL & JSON Schema foundation, `vigie schema` |
| M3 | ✅ | `vigie test` — template tier: full matcher library, matrix/cases, snapshots, helper tests |
| M4 | 🔜 | `vigie validate` (kubeconform), SARIF/TAP reporters, CI annotations |
| M5 | 🔜 | Distribution: Helm plugin, install scripts, pre-commit hook manifest |
| M6–M8 | 🔜 | `vigie test-apply` — envtest (api), e2e (kind/k3d), simulated tiers |
| M9 | 🔜 | `vigie doctor`, docs site, `watch`/`--changed` |

---

## License

Apache 2.0

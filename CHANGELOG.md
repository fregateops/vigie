# Changelog

All notable changes to this project are documented in this file.
## [0.6.0] - 2026-11-20

### Features

- *(cluster)* Add new cluster backend from kubeconfig
- *(deps)* Add test dependencies DAG resolution and installer from multiple sources
- *(runner)* Add LifecycleHook to run commands between tests
- *(matcher)* Add live kubernetes event matcher
- *(matcher)* Add live http client matcher
- *(matcher)* Add live pod log contain matcher
- *(matcher)* Add live kubernetes waitfor matcher
- *(runner)* Wire test against kubeconfig with dependencies hooks and live matcher support
- *(backend)* Add kind cluster backend
- *(backend)* Add k3d cluster backend and procutil helper
- *(doctor)* Add shared external-tool resolver
- *(test)* Add --kind-binary/--k3d-binary flags and download policy
- *(test)* Thread testApply.cluster.extraArgs to node backends
- *(config)* Generate and enforce a JSON Schema for .vigie.yaml
- *(dsl)* Add capabilities based resolution for matcher and tier
- *(dsl)* Publish matcher capabilities and the tier map in the schema
- *(dsl)* Declare a suite's tier, and remove cluster block
- *(report)* Report on assert the rendered/live into pretty, json, junit and tap reporters
- *(test)* Detect helm-unittest files and report them as skipped
- *(cluster)* Load locally built images into kind/k3d with --load-image

### Bug fixes

- *(test)* Fail a run whose every test skipped, and name a usable flag
- *(test)* Skip cluster matchers at the template tier, refuse named files
- *(apply)* Skip suites whose deps or hooks the tier cannot provide
- *(deps)* Resolve dep paths against the test file, not the working dir
- *(render)* Render like helm install and honour every test input
- *(test)* Report the render error instead of "no documents were rendered"
- *(lint)* Attribute findings to their template and honour ignore globs
- *(schema)* Fail when a schema can't be fetched, check strictly, name unchecked kinds
- *(test)* Report suites a tier cannot run as skipped instead of dropping them
- *(test)* Report a test file that fails to load as a failed suite
- *(snapshot)* Fail on a missing snapshot in CI instead of writing it
- *(test)* Fail an assertion whose document selection is ambiguous
- *(test)* Select a document only for assertions that read one
- *(snapshot)* Only compare snapshots unless -u is passed
- *(cluster)* Cancel cleanly on Ctrl-C/SIGTERM and stop live waits
- *(cluster)* Run chart hooks and uninstall each test's release
- *(cluster)* Select the kubeconfig context explicitly and announce the target
- *(hooks)* Run test-file commands with an allowlisted environment
- *(deps)* Never modify or delete objects vigie did not create
- *(cluster)* Explain cluster-tier failures and stop waiting after a failed install
- *(cluster)* Report a suite whose setup fails instead of aborting the run
- *(log)* Show warnings at default verbosity
- *(lookup)* Mask Secret values in timeout messages
- *(cluster)* Delete a cluster whose creation was interrupted or failed
- *(cli)* Trap Ctrl-C only for cluster runs, where there is cleanup to do
- *(build)* Split procutil's process-group attribute per platform
- *(deps)* Bump controller-runtime to v0.25.3 for Windows builds

### Refactor

- *(cluster)* Drive kind via external CLI instead of the library
- *(cluster)* Resolve the k3d CLI through the shared resolver
- *(config)* Unify test and testApply into one test.cluster model
- *(dsl)* Remove the dead per-test tier and tags fields
- Drop comments that restate names and a redundant nil guard
- Drop comments that restate a name

### Documentation

- *(dsl)* Document tier on test files and cleanup doc
- Document schemas, test inputs, selection and snapshot behaviour
- *(readme)* Document installation, cluster tests and CI usage

### Testing

- *(basic chart)* Refactor basic chart to be deployable and add integration tests
- *(basic chart)* Discover every test file from one tests/ root
- *(fixture)* Make the test-failures Deployment schema-valid

### Build

- *(pre-commit)* Add gitleaks pre-commit step

### Miscellaneous

- Update Makefile smoke tests
- Add non-blocking e2e smoke for kind and k3d
- Update to go 1.27
- Run the integration tests on k3d and build every release target
- Cross-build only windows and darwin, in a cached job of its own

### Style

- *(runner)* Gofmt validate.go

## [0.5.0] - 2026-08-13

### Features

- *(dist)* Add Helm plugin manifest and install hooks
- *(dist)* Add consumer-facing pre-commit hook manifest
- *(kubeclient)* Add in-memory REST client getter for envtest
- *(doctor)* Add envtest binary bootstrap (KUBEBUILDER_ASSETS → setup-envtest → download)
- *(cluster)* Add Backend abstraction and implement envtest backend
- *(matchers)* Add applies/rejected/lookup apply-tier matchers
- *(runner)* Add apply-tier runner (envtest install + assert)
- *(test)* Wire envtest apply test on test command
- *(test)* Unify --match, --fail-fast, and -u across both tiers
- *(reporter)* Add support for json output
- *(cli)* Kube-version matrix, improve flags and update README

### Miscellaneous

- Add CLI smoke test

## [0.4.0] - 2026-08-10

### Features

- *(validate)* Add kubeconform validator
- *(report)* Add SARIF 2.1.0 and TAP v13 report format support
- *(validate)* Add validate command with doc
- *(test)* Run kubeconform per test by default

## [0.3.0] - 2026-08-09

### Features

- *(path)* Add path expressions parser
- *(scehma)* Add vigie test DSL types and schema generation
- *(dsl)* Add schema parser and validator
- *(schema)* Add vigie schema command
- *(render)* Add CallHelper for helper-template
- *(matchers)* Add template test matchers
- *(runner)* Add first test runner
- *(test)* Wire  command
- *(matchers)* Add more complex matcher like CEL expression, schema and all/anyOf composite
- *(matrix)* Add support of matrix cases with values interpolation
- *(snapshot)* Add snapshot macher and store
- *(report)* Add junit output reporter

### Bug fixes

- *(schema)* Give resolvable schema ID
- *(config)* Make config parsing strict
- *(dsl)* Surface offending key + location in schema errors
- *(path)* Support quoted bracket keys for dotted/slashed map keys
- *(test)* Fail on run warnings with exit 5; add --pass-on-warning

### Refactor

- *(report)* Unify lint and test reporter

### Documentation

- Add config example and usage in README
- Add usage examples to commands
- *(readme)* Add editor autocomplete section

### Testing

- *(runner)* Add testdata charts for test command integration tests
- Add vigie config to basic test chart
- *(snapshot)* Commit golden snapshot use for tests
- Add simple smoke test makefile target

## [0.2.0] - 2026-08-08

### Features

- *(cel)* Wrap CEL evaluator from cel-go
- *(render)* Add helm chart rendering using helm v3 engine
- *(lint)* Add lint types and base runner
- *(lint)* Add linter rule engine and rules dataset
- *(lint)* Add rules loader
- *(lint)* Add helm3 linter adapter
- *(report)* Add lint result reporter with CI detection
- *(lint)* Wire vigie lint command

### Refactor

- *(lint)* Cleanup executor test

### Testing

- *(lint)* Add lint command tests with testdata charts

## [0.1.0] - 2026-08-08

### Features

- Bootstrap vigie project with cobra init and dev env

### Bug fixes

- *(release)* Ignore transient release_notes.md
- *(release)* Make goreleaser replace GH release body
- *(release)* Write release notes and verify it
- *(release)* Remove changelog config from goreleaser

### Documentation

- Add Apache2.0 license
- Init README.md

### Miscellaneous

- Setup main/PR CI and git hooks
- Add Makefile
- Setup release and changelog process
- Setup dependabot

<!-- generated by git-cliff -->

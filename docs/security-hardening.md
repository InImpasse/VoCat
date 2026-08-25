# Security Hardening Workflow

This fork keeps upstream history easy to inspect while preserving local security controls.

## Branches

- `master`: fork baseline; do not deploy it directly after an upstream pull.
- `security-hardening`: reviewed long-lived security baseline and deployment source.
- `merge/upstream-YYYYMMDD`: temporary integration branch created from `security-hardening`.

Configure remotes once:

```bash
git remote add upstream https://github.com/MengMengCode/VoCat.git
git fetch --prune upstream
```

For each upstream intake, fetch `upstream`, create a temporary merge branch from `security-hardening`, and merge the reviewed `upstream/master` commit with `--no-ff`. Review the full upstream range and every conflict before merging the integration branch back into `security-hardening`; never deploy the integration branch or an upstream build directly.

## Merge Gate

1. Record the old and new upstream SHAs and review `git diff --stat` plus security-sensitive paths.
2. Re-run authentication, update, extension, proxy, notification, eSIM, and deployment regression tests affected by the range.
3. Run `go test ./...`, native `go test -race -p 1 ./...`, `go vet ./...`, `npm ci`, `npm test`, `npm run build`, and both full and production-only `npm audit` checks.
4. Review dependency and base-image changes. Update version and digest together; never accept a floating release image.
5. Inspect the pending diff for credentials, private endpoints, generated assets, and unrelated changes.
6. Confirm license, geographic, permitted-use, evaluation-period, and SIM restrictions were not bypassed or weakened. Production self-update, third-party plugins, Export Proxy, and privileged containers must remain unavailable; the production unit must clear inherited updater repository credentials.
7. Commit the reviewed result. Pushes to `security-hardening` and `merge/upstream-*`, and pull requests targeting `security-hardening`, run the candidate CI described below. For a later production release, run `scripts/build-hardened.sh amd64`, verify its manifest and `SHA256SUMS`, and record the emitted `artifact index sha256` through a trusted out-of-band channel before signing a deployment tag.

The build script captures one commit, exports that commit with `git archive`, and runs every gate inside versioned, digest-pinned containers. Working-tree changes are reported and ignored. The exported source is never mounted writable and is hashed before and after all container work. The frontend is built in a separate `web` copy; only a regular-file `dist` tree (at most 2,000 files and 50 MiB) is copied into a separate Go build tree. Every fetch, audit, lifecycle, scanner, test, build, SBOM, and manifest phase receives a private HOME/TMP and a dedicated output directory. Reports from phases that execute project or dependency code are captured by the host outside the container mounts. The host validates each exact file inventory before assembling the release, so a background process or later phase cannot rewrite an earlier report or the release binary. Shared dependency, tool, and scanner caches live under `VOCAT_BUILD_CACHE_ROOT`; no container receives the cache temporary root as an alias to source or another phase. Release artifacts live under `dist/hardened/<commit>`. Production deployment requires both `--expected-commit <40-hex>` and `--expected-index-sha256 <64-hex>`; the latter must be the builder's `SHA256SUMS` hash carried through a trusted channel independent of the artifact transfer. Recomputing the expected index hash from the destination artifact directory destroys that authenticity check.

The committed-source gate fails on any Gitleaks finding, high or critical npm advisory, reachable `govulncheck` source or binary finding, Go module download or `go mod verify` failure, frontend test or build failure, Go test or race failure, vet failure, malformed report, or build failure. Network access is limited to tool and dependency retrieval, npm advisory queries, and the `govulncheck` vulnerability database. npm installs the exact lockfile with lifecycle scripts disabled; those scripts then run in a separate offline container that can write only `node_modules`, while tests use that tree read-only and the frontend build receives a disposable private copy. After `go mod verify`, every Go consumer mounts the module cache read-only. Go test, vet, source scan, release build, binary scan, and race test each use a distinct build cache, HOME, TMP, GOPATH, container, and report output; test, vet, build, and race phases are offline. Go test, vet, release build, and race compilation run with cache verification enabled after the module cache has been checked against `go.sum`. Source `govulncheck` is the exception: its `go/packages` loader must read export data generated during the same command, which `gocacheverify` deliberately bypasses, so each release gives that scanner a new build-scoped GOCACHE below `VOCAT_BUILD_CACHE_ROOT` and never reuses it. Normal and race tests run in the digest-pinned Bookworm Go image so deployment-script tests have Bash and Python; the exact jq 1.8.1 binary is extracted from its separate digest-pinned, shell-less image and mounted read-only. Those tests also receive minimal synthetic passwd/group and Ubuntu 24.04 release files, so release evidence does not depend on or disclose the builder's account name or host distribution. The builder parses the delivered binary's own `go version -m` output and requires Go 1.26.7 plus the requested Linux architecture before packaging it; schema 2 records both native test toolchains, jq, the frontend test/build gates, and the same binary metadata report. For symbol-level `govulncheck` SARIF, `error` results are reachable and block release; `warning` and `note` results are retained for review because they identify imported packages or dependency-only modules without a reachable vulnerable call. The release binary keeps the Go symbol table (only DWARF data is removed with `-w`) so binary `govulncheck` analyzes the actual delivered file without the conservative false positives caused by `-s`. The race suite uses the native build architecture with CGO in the pinned Bookworm toolchain; the release binary remains a `CGO_ENABLED=0` static `linux/amd64` or `linux/arm64` build. The artifact contains redacted secret-scan output, full and production npm audit JSON, frontend test/build logs, module download/verification logs, source and binary `govulncheck` SARIF, Go test/vet logs, `go version -m`, source and binary CycloneDX SBOMs, exact tool/image versions, a structured manifest, and checksums covering every regular artifact file except `SHA256SUMS` itself. During deployment, the trusted out-of-band index authenticates `SHA256SUMS`; the deployer then validates its paths, exact inventory, and every file checksum before parsing JSON. It independently revalidates the raw Gitleaks, npm, SARIF, and CycloneDX reports and requires their derived counts, fixed paths, thresholds, scan modes, toolchains, and integrity scope to match schema 2 exactly.

`.gitleaks.toml` extends the upstream Gitleaks rules without project-specific secret allowlists.

## Candidate CI

`.github/workflows/security-candidate.yml` runs the complete committed-source gate for Linux amd64 on eligible pushes and pull requests. It uses read-only repository permissions, disables checkout credential persistence, pins every referenced Action to a full commit SHA, and stores the complete schema 2 artifact directory for seven days. The uploaded bundle is temporary review evidence tied to `github.sha`; it is not a production release.

The candidate workflow does not create or update tags, GitHub Releases, or container images. It has no package, OIDC, or repository write permission and receives no publishing credential. Tagging and release publication require a separate, explicit request after the candidate SHA has passed review; CI success alone never authorizes either action.

## Residual Supply-Chain Risk

The `release`, `docker`, and carrier-bundle sync workflows are manual, read-only, fail-closed guards. They contain no third-party Action references, do not run on tag pushes, and terminate with an error before publishing or synchronizing anything. They are not build, release, container-distribution, or third-party-data intake paths.

Those guards still start a GitHub-hosted `ubuntu-latest` runner when manually dispatched. Their current commands only emit the disabled message and exit, so that mutable runner image is not trusted with source checkout, artifacts, write permissions, or secrets. The candidate workflow also uses a mutable GitHub-hosted runner, but pins its Actions and all build/scanner images, grants only read access, and reproduces the committed-source gates before uploading temporary evidence. A candidate is not production-approved until a later explicit release decision and the separate deployment and real modem acceptance gates pass.

# Project Memory

## Repository Model

- This repository is a maintained fork. Keep `upstream` pointed at `MengMengCode/VoCat` and `origin` pointed at the fork.
- Keep `security-hardening` as the long-lived deployment branch. Merge upstream only through short-lived `merge/upstream-YYYYMMDD` branches using `--no-ff` after all gates pass.
- Never mix unrelated local work into an upstream sync or security release.

## Security Release Rules

- Release artifacts must come from a reviewed, committed SHA, not the working tree. Use `scripts/build-hardened.sh` after the release commit exists.
- Keep source compatibility at Go 1.25 while building releases with the explicit Go 1.26.7 toolchain.
- Treat Docker image digests, dependency lockfiles, scanner versions, SBOMs, CI tool versions, and generated checksums as security controls.
- Require manifest schema 2 and retain the secret scan, full and production npm audits, frontend test/build logs, normal/race Go tests, vet, source/binary vulnerability scans, source/binary SBOMs, tool versions, actual binary Go metadata, and whole-artifact checksums as release evidence. The binary metadata itself must prove Go 1.26.7 and the requested Linux architecture.
- Keep dependency retrieval, lifecycle code, tests, scans, builds, and report generation in separate containers with private HOME/TMP/GOPATH/GOCACHE/output paths. Mount the verified Go module cache read-only after download and verification.
- Keep `GODEBUG=gocacheverify=1` on Go test, vet, release build, and race phases. Source `govulncheck` is the documented exception because `go/packages` must read newly generated export data; give it a fresh build-scoped GOCACHE below the configured build cache root and never reuse that cache across builds.
- Never put credentials, private endpoints, SIM secrets, notification tokens, or production identifiers in tracked files, logs, manifests, or build arguments.
- Never bypass or weaken license, geographic, permitted-use, evaluation-period, or SIM eligibility restrictions. Keep authorization documents and their sensitive terms outside the repository.
- Never restore production self-update, third-party plugins, Export Proxy, or privileged containers. Production units must clear updater credentials even if systemd global environment state attempts to inject them.
- A passing local build is a candidate only. Deployment and real modem acceptance are separate gates.
- Candidate CI may run on eligible pushes and pull requests with read-only permissions and seven-day artifact retention. It must never create tags, GitHub Releases, or container images; those actions require a later explicit user request.
- Production deployment must retain the root-only artifact snapshot, private deployment lock, service-identity online SQLite snapshot, live DB/WAL/SHM metadata checks, isolated `vocat-preflight` account, constrained transient preflight unit, consistent database backup, and binary-plus-database rollback. After rollback, revalidate the previous release's MainPID, executable, exclusive 7575 listener ownership, and `/readyz`; a successful `systemctl start` alone is not recovery. Do not weaken these checks to make an upgrade pass.
- Production deployment requires the reviewed 40-hex commit and the `SHA256SUMS` index hash through a trusted out-of-band channel independent of artifact transfer. Never derive either expected value from the destination artifact directory.
- Authenticate the complete artifact inventory and every file checksum before parsing JSON. The deployer must revalidate raw Gitleaks, npm audit, govulncheck, and SBOM evidence and bind the derived values to the exact schema 2 paths, toolchains, thresholds, and integrity scope.
- VM validation must compare inactive and live libvirt state, keep installation media read-only and eject it after installation, and reject unexpected storage, network, PCI, or USB devices.
- `CAP_NET_ADMIN` and `CAP_NET_RAW` remain an explicit residual risk. Guest nftables is an ingress control, not a security boundary after a VoCat RCE; durable containment requires an external ACL or a future narrowly scoped privileged helper.

## Current Scope

- The supply-chain batch pins build and scanner images, fixes the known `nanoid` advisory, and adds a committed-source release gate. It includes normal/race tests, vet, npm audits, secret scanning, source/binary Go vulnerability scans, source/binary SBOMs, build metadata, and whole-artifact checksums.
- Preserve three reviewable batches: build chain, authentication/API, and deployment. Keep each fix with its regression test.
- See `docs/security-hardening.md` for the recurring upstream merge and release gate.
- Do not record host models, hostnames, account names, local paths, network topology or addresses, interface names, storage mount points, or live workload inventory in tracked files or commit metadata. Supply host-specific values only through private local parameters.
- Do not create a production USB allowlist, claim firmware visibility, or upgrade firmware until the exact identity and AT path can be verified locally.

# Changelog

All notable changes to LOOPZE are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **License: relicensed from Elastic License 2.0 (ELv2) to AGPL-3.0-or-later.** Strong copyleft + § 13 (SaaS clause) prevents embedding into proprietary products. Copyright transferred to Dennis Bleul personally.
- All Go source-file headers updated to the new license.
- All German documentation, source comments, and UI strings translated to English (~12,500 lines across 46 files) in preparation for public release.

### Added
- `NOTICE` file with copyright notice and source-code URL.
- License + source-code link surfaced in the Settings view to satisfy AGPL § 13 (network users must be able to obtain the source).
- `CONTRIBUTING.md` with PR workflow and an inbound=outbound licensing clause that preserves dual-licensing flexibility.
- `SECURITY.md` with private vulnerability-reporting process and disclosure timeline.
- `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1).
- GitHub Actions workflow (`.github/workflows/ci.yml`) running `go vet` / `go build` / `go test` and a frontend type-check + build on push and pull request.

### Removed
- Accidentally tracked `opcua-smoke` smoke-test binary (7.5 MB ELF) removed from the index.

---

> Earlier development history is preserved in git but is not retroactively
> documented here — this changelog starts with the public release preparation.

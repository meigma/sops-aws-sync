---
id: 005
title: Phase 4 GitHub Action wrapper
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002, 003, 004]
---

## Goal

Review session 001's authoritative V1 design and delivery plan, then implement,
verify, review, and land Phase 4: the thin Node 24 TypeScript GitHub Action
around the completed Go CLI.

## Outcome

The goal was met. [PR #10](https://github.com/meigma/sops-aws-sync/pull/10)
was reviewed, approved, and squash-merged into `master` as
`8e9db4cff42ab24eb0b04873fb69d95ac269286b`. The repository now has a
committed Node 24 Action that translates a closed typed input surface into an
explicit shell-free Go CLI invocation, installs an exact CLI release only after
checksum and GitHub attestation verification, re-verifies cache hits, and
publishes only validated redacted outputs and summaries.

Human review found that current `gh attestation verify` rejects even public
verification without authentication. The reviewed head
`bce41704d8cdfe6e0f78c30cfa6545ae6bc7ee93` fixed the default caller path by
defaulting the optional `github-token` input to `${{ github.token }}`, masking
it before installation, and failing before `gh` if the runner does not supply
the metadata default. Hosted CI, GitHub Pages, and Kusari Inspector passed on
that exact head.

## Key Decisions

- Keep the Action a transport adapter around the Go binary -> Git, SOPS, AWS,
  ownership, retry, and reconciliation policy remain in the tested Go core.
- Install only the Action's embedded exact semantic CLI version -> mutable tags
  and caller-selected arbitrary binaries cannot change execution behavior.
- Require checksums plus the complete immutable `gh attestation verify`
  identity policy for both downloads and cache hits -> cached bytes receive the
  same provenance treatment as newly downloaded bytes.
- Isolate GitHub CLI configuration while explicitly supplying the masked
  workflow token -> unrelated ambient GitHub credentials cannot influence
  verification, but the omitted-input public workflow remains authenticated.
- Parse every input into validated types and build argv without a shell or
  arbitrary argument escape hatch -> the Action cannot widen the CLI contract.
- Accept only the strict redacted report schema for outputs and summaries ->
  resource names, source paths, secret values, and raw adapter failures remain
  outside the public workflow surface.
- Restrict Go gates to repository-owned `cmd` and `internal` packages -> npm
  dependencies that happen to contain Go source do not enter Go test discovery.

## Changes

- `action.yml` - added the Node 24 Action metadata, closed input contract,
  workflow-token default, stable outputs, and committed bundle entrypoint.
- `action/src` - added typed input parsing, explicit argv construction, exact
  release resolution, checksum and attestation verification, verified caching,
  CLI execution, report validation, and safe output/summary orchestration.
- `action/__tests__` - added 43 tests covering inputs, command construction,
  installation, provenance policy, cache behavior, report safety, exit mapping,
  token handling, and a functional invocation of the real Go binary.
- `action/dist` - committed the Rollup-generated JavaScript bundle and source
  map required for direct Action execution.
- `action/package.json`, `action/package-lock.json`, and Action tooling - added
  the pinned TypeScript Action development, lint, test, coverage, dependency,
  audit, and packaging surface derived from the recorded upstream baseline.
- `mise.toml`, `mise.lock`, `moon.yml`, and Moon workspace configuration -
  pinned Node 24.4.0 and integrated Action checks with the repository gate.

## Open Threads

- Phase 5 remains: hardening, operator and security documentation, release
  publication, branch/CODEOWNERS policy, protected AWS workflow evidence, and
  exact-merged-SHA acceptance.
- Phase 5 documentation should state that callers may omit `github-token`
  because it defaults to `${{ github.token }}`; current GitHub CLI still needs
  authenticated access for public attestation verification. Private producer
  repositories require an explicitly scoped replacement token.

## Lessons

- Public GitHub release visibility does not imply unauthenticated GitHub CLI
  attestation verification; reproduce the installed `gh` behavior instead of
  treating API visibility as the authentication contract.
- A metadata-contract test is necessary when runtime correctness depends on an
  Action input default that unit-level `core.getInput` fakes do not provide.
- Committed Action bundles need a staged-dist parity gate because a correct
  TypeScript source change is not sufficient for JavaScript Action consumers.

## References

- [PR #10: feat(action): add verified CLI adapter](https://github.com/meigma/sops-aws-sync/pull/10)
- [Hosted CI run](https://github.com/meigma/sops-aws-sync/actions/runs/29789609945)
- [GitHub Pages run](https://github.com/meigma/sops-aws-sync/actions/runs/29789609952)
- Merge commit `8e9db4cff42ab24eb0b04873fb69d95ac269286b`
- Reviewed head `bce41704d8cdfe6e0f78c30cfa6545ae6bc7ee93`
- `.journal/001/DESIGN.md` - authoritative V1 design
- `.journal/001/PLAN.md` - five-phase delivery roadmap
- `.journal/004/SUMMARY.md` - complete Go CLI baseline
- `.journal/005/NOTES.md` - execution log and review evidence

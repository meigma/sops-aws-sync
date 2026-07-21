---
id: 006
title: Phase 5 hardening and release readiness
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002, 003, 004, 005]
---

## Goal

Review session 001's authoritative design and plan, then execute Phase 5:
repository hardening, release integration and rehearsal, operator documentation,
repository policy, and final V1 acceptance evidence.

## Outcome

The goal was partially met. [PR #11](https://github.com/meigma/sops-aws-sync/pull/11)
was reviewed, approved, and squash-merged into `master` as
`b67210d287faa642f7b4e1d7e73da438b5253abb`. The repository now has a complete
Go, Action, bundle-drift, documentation, race, vulnerability, and workflow gate;
paired CLI/Action release-version automation; a full pre-publish binary/SBOM
rehearsal; CODEOWNERS and desired branch policy; and V1 operator, security, and
release-verification documentation.

The reviewed head `4193b661c8d0bc833af956a76137449f7b0e754b` passed all PR
checks, including the binary release dry run. The squash-merged SHA passed the
complete default-branch CI gate. Release Please then failed before execution
because the GitHub App client ID/private key are not configured, and Pages failed
after a successful docs build because the repository has no GitHub Actions Pages
site. No tag or release was published. At the user's direction, AWS acceptance
remains a manual functional test and is not encoded as repository automation.

## Key Decisions

- Keep the CLI and Action on one immutable version -> Release Please updates
  `action.yml`, the npm package, and both package-lock root version fields, while
  dependency policy checks enforce the coupling.
- Run release rehearsal on every PR -> supported binaries, checksums, SBOMs,
  committed Action bundle parity, asset staging, and version output are proved
  without publishing.
- Add race, vulnerability, and workflow linting to the root gate -> Phase 5
  hardening becomes part of every normal repository check rather than a separate
  operator convention.
- Upgrade Go from 1.26.4 to 1.26.5 -> the new vulnerability gate found a called
  standard-library ECH privacy issue fixed by the patch release.
- Build the private main module from the checked-out commit -> GoReleaser's main
  module proxy cannot resolve a synthetic local-only tag in a private repository.
- Keep AWS sandbox acceptance manual -> the exercise is one-off functional
  evidence, not a reusable or dispatchable privileged workflow.
- Stop short of publication -> merging and closing the session did not authorize
  creating release credentials, enabling paid repository controls, publishing a
  release, or running the manual AWS exercise.

## Changes

- `moon.yml`, `go.mod`, `go.sum`, `mise.toml`, and `mise.lock` - added pinned
  `actionlint` and `govulncheck` tools, race/vulnerability/workflow gates, and Go
  1.26.5.
- `.github/workflows/release-dry-run.yml`, `.github/workflows/release.yml`, and
  `.goreleaser.yaml` - expanded and pinned the release rehearsal, paired-version
  check, bundle verification, supported binary/SBOM staging, and smoke test.
- `release-please-config.json`, `action.yml`, `action/src`, Action tests, and
  `action/dist` - moved the paired CLI default into release-managed metadata and
  preserved the committed bundle contract.
- `.github/CODEOWNERS` and `.github/repository-settings.toml` - recorded code-owner
  review, approval, thread-resolution, squash-only, and required-check policy.
- `README.md` and `docs/docs` - documented installation, configuration, Action
  trust and authentication, IAM/KMS, ownership, consistency, interruption,
  recovery, logging, and artifact verification.
- `.golangci.yml` - kept Go formatting scoped away from generated documentation
  and bundled Action trees so parallel Moon tasks do not race.

## Open Threads

- Configure the release GitHub App client ID/private key, then rerun Release
  Please. The token action also warns that the legacy `app-id` input is
  deprecated in favor of `client-id`.
- Enable GitHub Pages with GitHub Actions as its build source, then rerun the
  Pages workflow.
- Activate the desired CODEOWNERS/ruleset policy after the private repository's
  plan or visibility supports the required protection APIs; current requests
  return HTTP 403.
- Publish and independently verify one immutable release from an explicitly
  authorized merged commit. No release or tag exists yet.
- Run the exact-release manual AWS sandbox functional test for create, no-op,
  update, scheduled deletion, restore/follow-up update, safe output, and observed
  convergence using caller-owned authentication. Do not turn it into repository
  automation without an explicit design decision.

## Lessons

- Hosted release rehearsal exposed two issues local checks could not reproduce:
  ShellCheck saw an unused polling variable, and main-module proxying could not
  resolve a private synthetic tag.
- A successful documentation build does not prove Pages deployment readiness;
  the repository-level Pages site must already be enabled for GitHub Actions.
- Acceptance evidence and durable automation are different products. A valuable
  manual functional proof should remain manual when repeated privileged
  execution is not an operational requirement.

## References

- [PR #11: feat: complete V1 hardening and acceptance](https://github.com/meigma/sops-aws-sync/pull/11)
- Merge commit `b67210d287faa642f7b4e1d7e73da438b5253abb`
- Reviewed head `4193b661c8d0bc833af956a76137449f7b0e754b`
- [Final PR CI](https://github.com/meigma/sops-aws-sync/actions/runs/29792924356)
- [Final PR release dry run](https://github.com/meigma/sops-aws-sync/actions/runs/29792924338)
- [Merge-SHA CI](https://github.com/meigma/sops-aws-sync/actions/runs/29793089486)
- [Release Please configuration failure](https://github.com/meigma/sops-aws-sync/actions/runs/29793089455)
- [Pages configuration failure](https://github.com/meigma/sops-aws-sync/actions/runs/29793089429)
- `.journal/001/DESIGN.md` - authoritative V1 design
- `.journal/001/PLAN.md` - five-phase delivery roadmap
- `.journal/005/SUMMARY.md` - verified Action baseline

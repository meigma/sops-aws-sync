---
id: 006
title: Phase 5 hardening and release readiness
started: 2026-07-20
---

## 2026-07-20 17:23 — Kickoff
Goal for the session: Review session 001's authoritative design and plan, then execute Phase 5 of the delivery plan.
Current state of the world: Phases 2 through 4 are merged on `master`; the durable Go reconciliation core, complete lifecycle CLI, and verified Node 24 GitHub Action adapter are in place. Phase 5 hardening, documentation, publication, protected AWS workflow evidence, and exact-merged-SHA acceptance remain.
Plan: Re-read the session 001 artifacts, inspect the current repository against the Phase 5 checklist, implement the smallest coherent hardening and documentation slices, verify locally and in hosted workflows, and pause at any explicit publication or approval gate.

## 2026-07-20 17:46 — Phase 5 authority and live audit
Reviewed `.journal/001/DESIGN.md` and `.journal/001/PLAN.md` in full. The bounded pre-merge slice is release-version coupling, complete release rehearsal coverage, CODEOWNERS and desired repository policy, a protected serialized Action-to-AWS acceptance workflow, and operator/security documentation. Post-merge publication and exact-SHA AWS evidence remain approval-gated.

Live GitHub inspection found the private repository has no environment, Actions variables, Actions secrets, or active protection configuration. GitHub rejects both branch-protection and ruleset APIs with HTTP 403 (`Upgrade to GitHub Pro or make this repository public`). The repository settings manifest can record the required policy, but applying branch/code-owner protection is externally blocked until the account or visibility constraint changes. The release GitHub App variable/private-key are also not configured, and the protected AWS workflow will require a caller-owned OIDC role plus environment configuration.

Created isolated implementation worktree `.wt/phase5-v1-hardening-acceptance` on branch `phase5/v1-hardening-acceptance` from fetched `origin/master` at `8e9db4c`.

## 2026-07-20 17:45 — Pre-merge implementation complete
Committed the Phase 5 pre-merge slice as `db3ab62cb967e12263b47b336fbe26e8ecea9077` (`feat: complete V1 hardening and acceptance`). The change couples the Action metadata, npm package, lockfile, and repository release version through Release Please; expands the root gate with race, vulnerability, and workflow checks; exercises release binaries, SBOMs, and bundle drift on every PR; adds CODEOWNERS and desired protection policy; adds the protected serialized AWS sandbox workflow; and replaces phase-status documentation with the V1 operator contract.

Verification on the reviewed commit passed: all 25 `root:check` tasks, all 44 Action tests plus coverage and bundle drift, Go race tests, `govulncheck` with no called vulnerabilities, `actionlint`, strict MkDocs build, dependency policy, and `git diff --check`. Go was raised from 1.26.4 to 1.26.5 because the initial vulnerability scan identified a standard-library ECH privacy issue fixed in 1.26.5.

The remaining work is hosted PR evidence followed by the explicit review gate. Exact merged-SHA release and AWS evidence cannot run before merge, and live branch/environment protection remains blocked by the repository's private-plan API restriction plus absent release-App and caller-owned AWS OIDC configuration.

## 2026-07-20 18:08 — PR evidence green; paused for review
Opened PR #11, `feat: complete V1 hardening and acceptance`, targeting `master`. Hosted evidence exposed and corrected two rehearsal-only defects: ShellCheck rejected an unused polling-loop variable, and GoReleaser's main-module proxy could not resolve a private repository at a synthetic local-only tag. Follow-up commits `0691a6b` and `5329742` respectively fixed those issues without changing the V1 contract.

The final reviewed head is `5329742056624d31ecfe0a3936161856318cff22`. PR checks are green: CI, Binary Release Dry Run, GitHub Pages, and Kusari Inspector all succeeded; the Pages deployment job was expectedly skipped for the pull request. The hosted release dry run took 12m9s and proved the four supported binaries, checksums, SBOMs, committed Action bundle, asset staging, and binary version smoke test. The exact open-source GoReleaser 2.17.0 rehearsal was also reproduced locally, including checksum verification of the downloaded GoReleaser binary; its temporary tag was removed.

PR #11 is open, mergeable, and clean. Stop here for user review. Do not squash merge, publish a release, configure account billing/visibility, create GitHub credentials, or run AWS acceptance without explicit authorization and the missing external configuration.

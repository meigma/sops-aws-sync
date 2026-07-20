---
id: 002
title: Phase 1 committed-source proof
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001]
---

## Goal

Review session 001's authoritative V1 design and delivery plan, then execute
Phase 1: a disposable proof that one regular `.sops.json` blob from an exact
Git commit becomes one pure `create` decision without AWS access.

## Outcome

The goal was met. The spike proved the complete Phase 1 source-to-domain path,
the user reviewed and accepted the evidence, and draft PR #7 was closed without
merge exactly as the plan requires. Its remote branch and isolated Worktrunk
worktree were deleted. The default `master` branch remains unchanged at the
template's initial commit `3b1848b97e4a9e35d239c5f15ad987ad438766cd`;
durable product implementation therefore still begins in Phase 2.

No design conflict or ambiguity was found.

## Proof and Findings

- The end-to-end functional test created a temporary Git repository, committed
  one encrypted fixture, replaced the working-tree copy with conflicting data,
  and still produced the expected plan from the committed blob. This proved
  desired state does not depend on the working tree or Git executable.
- `github.com/getsops/sops/v3/decrypt.DataWithFormat` with JSON format decrypted
  a hermetic age-backed fixture in process and verified its MAC. A separately
  tampered MAC failed closed.
- Strict plaintext validation rejected a non-object top level, duplicate nested
  members, and trailing JSON. RFC 8785 canonicalization produced the expected
  deterministic nested object and normalized `1.0` to `1`.
- Mapping `secrets/production/database.sops.json` beneath source root `secrets`
  and prefix `/acme/payments` produced exactly
  `/acme/payments/production/database` plus the source-path SHA-256 identity.
- The pure planner was independent of Git and SOPS and returned exactly one
  deterministic `create` against absent observed state. Its report contained
  only schema and count fields; fixture plaintext, source paths, and secret
  names were absent.
- Production source contained no direct AWS SDK import, Secrets Manager client,
  or mutation path. SOPS itself brings provider SDKs, including AWS KMS, into
  the transitive module graph; that is library dependency breadth, not an AWS
  reconciliation adapter or an AWS call in this proof.
- Implementation-time compatible pins were go-git `v5.19.1`, SOPS `v3.13.2`,
  JCS `v1.0.1`, and Testify `v1.11.1`.

## Verification

- Exact reviewed proof commit:
  `5bcae2d65a934e234ac4ce345b4cc0c737a96080`.
- Focused race gate passed:
  `mise exec -- go test -race ./internal/domain ./internal/adapters/sopsdecrypt ./internal/application`.
- Complete repository gate passed:
  `MOON_TOOLCHAIN_FORCE_GLOBALS=true mise exec -- moon run root:check --force`.
- Hosted checks passed on the exact proof commit: CI in 4m32s, GitHub Pages in
  29s, and Kusari Inspector in 2m10s. Release dry-run jobs skipped as expected
  for the draft proof.
- PR #7 was confirmed closed at `2026-07-20T19:25:58Z` with `mergedAt: null`.

## Key Decisions

- Kept the proof disposable and source-to-domain only because Phase 1 exists to
  learn at the riskiest boundary, not to pre-build the durable CLI or AWS path.
- Preserved the hexagonal dependency rule even in spike code: go-git and SOPS
  remained adapters around application ports, while mapping and planning stayed
  in a standard-library-only domain.
- Used a committed public test age identity and encrypted sentinel fixture so
  decryption and MAC behavior were hermetic and required no cloud credentials.
- Closed rather than merged the accepted PR because session 001's plan makes
  that disposition a Phase 1 success criterion.

## Changes

- Closed draft PR #7 without merge; the PR retains the complete spike diff and
  reviewable evidence at head `5bcae2d`.
- Deleted remote and local branch `phase1/committed-source-proof` and removed its
  Worktrunk worktree.
- Left `master`, repository identity, CLI, AWS integration, Action, release
  automation, and all other durable product surfaces unchanged.
- `.journal/002/NOTES.md` recorded execution checkpoints and final disposition.
- `.journal/TECH_NOTES.md` now points future agents to the accepted proof and
  the Phase 2 starting state.

## Open Threads

- Phase 2 is next. Start it in a new journal session and a fresh implementation
  worktree from fetched `master`; reread session 001's `DESIGN.md` and `PLAN.md`
  before coding.
- Phase 2 is the first mergeable PR and must build the durable AWS vertical
  slice defined in the plan. Do not treat PR #7's closed spike as landed code or
  merge/reopen it; consult its diff only as executable evidence.
- Phases 3 through 5 remain unimplemented.
- Session 001's `DESIGN.md` remains the sole V1 authority. Stop for user
  direction if durable implementation evidence conflicts with it.

## Lessons

- A small committed-source spike retired the most important uncertainty without
  coupling the product to provisional code: go-git exact-tree reads, the stable
  SOPS binding, strict validation, JCS, and pure planning compose successfully.
- On this workstation, Moon's global Proto lookup can select Go 1.26.5 while
  mise supplies the pinned Go 1.26.4 `GOROOT`. Running Moon with
  `MOON_TOOLCHAIN_FORCE_GLOBALS=true` makes the system toolchain honor mise; do
  not change repository pins to work around that local environment collision.
- SOPS's provider-complete dependency graph makes a cold hosted Go gate slower
  than the template baseline; the observed CI run still completed successfully.

## References

- [Phase 1 draft PR #7](https://github.com/meigma/sops-aws-sync/pull/7)
- [Hosted CI run](https://github.com/meigma/sops-aws-sync/actions/runs/29771295131)
- [GitHub Pages run](https://github.com/meigma/sops-aws-sync/actions/runs/29771295995)
- [`../001/DESIGN.md`](../001/DESIGN.md)
- [`../001/PLAN.md`](../001/PLAN.md)
- [`../001/SUMMARY.md`](../001/SUMMARY.md)
- [`NOTES.md`](./NOTES.md)

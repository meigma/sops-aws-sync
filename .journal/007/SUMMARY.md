---
id: 007
title: Manual E2E release acceptance
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002, 003, 004, 005, 006]
---

## Goal

Design, refine, and execute a manual user-level acceptance test for the released
GitHub Action using a temporary private consumer repository and disposable AWS
sandbox. Exercise both happy paths and realistic mistakes, treat release trust
and safety as gates, resolve blockers found during execution, and remove all
temporary resources.

## Outcome

The goal was met. The initial release attempt found a genuine draft-resolution
blocker. [PR #12](https://github.com/meigma/sops-aws-sync/pull/12) was reviewed,
approved, and squash-merged as
`0894dfe568f9a1cb6df616d2cfb21260a4b7061f`, correcting the release handoff
without weakening its trust gates.

After the user corrected the OSS repository's visibility to public, GitHub
provenance succeeded and immutable prerelease
[`v0.1.1`](https://github.com/meigma/sops-aws-sync/releases/tag/v0.1.1) was
published. `gh release verify` validated all four binaries, four SBOMs, and the
checksum asset. Scenarios A–E then passed against a temporary private consumer
repository and Whizlabs AWS account, covering release verification,
non-mutating plans, ownership conflicts, serialized repeated runs, pinned
revisions, create/update/restore/delete lifecycle behavior, external-drift
repair, invalid plaintext rejection, log redaction, empty desired state, and
scope boundaries.

Cleanup completed: the consumer repository was deleted, the AWS sandbox was
destroyed and confirmed absent on a second attempt, and the local clone, age
key, plaintext scratch files, and isolated Whizlabs state were removed. The
producer remains clean and synchronized at the immutable prerelease commit.

## Key Decisions

- Stop at the exact-release/provenance gate rather than test an untrusted build
  -> acceptance needed to prove the same installation path real consumers use.
- Resolve drafts through the release-list API with `contents: write` and an
  exact tag match -> GitHub's by-tag endpoint exposes published releases, not
  the draft the workflow was required to find.
- Use the caller's normal workflow token after the producer became public -> a
  broad or separately managed producer-read token was unnecessary.
- Keep foreign same-prefix and out-of-prefix sentinels throughout the run ->
  ownership and scope safety were proved directly, not inferred from reports.
- Preserve a deliberate empty source root with `.gitkeep` -> Git cannot commit
  an empty directory, and a missing source root is correctly rejected before
  empty-state authorization is considered.
- Keep AWS acceptance manual -> the exercise validates real operator behavior
  without introducing a reusable privileged test workflow.

## Changes

- `.github/workflows/release.yml` - gave the resolver draft visibility and
  exact-matched the draft tag through the release-list endpoint.
- GitHub release configuration - enabled immutable releases and published
  verified prerelease `v0.1.1` after successful provenance.
- `.journal/007/MANUAL_E2E_TEST_PLAN.md` - recorded the refined five-scenario
  manual acceptance procedure and its final execution state.
- `.journal/007/EXECUTION_RESULT.md` - recorded scenario results, run IDs,
  safety observations, release evidence, and cleanup proof.

## Open Threads

- `v0.1.1` intentionally remains a prerelease; deciding whether and how to
  promote a stable release is separate release authorization.
- Operator documentation should mention that a deliberately empty committed
  source needs a tracked placeholder such as `secrets/.gitkeep`.
- Temporary consumer workflow URLs disappeared with the required repository
  deletion; `EXECUTION_RESULT.md` retains their historical run IDs and the
  observations captured before cleanup.

## Lessons

- A successful release rehearsal does not prove the live draft-to-artifact
  handoff; the real GitHub release visibility rules exposed the blocker.
- Private-repository artifact attestation availability depends on the GitHub
  organization plan, while the intended public OSS repository supports it.
- `whzbox create` prints disposable credentials unless output is suppressed;
  treat accidental output as exposure and destroy that sandbox immediately.
- Missing and intentionally empty Git source roots are distinct user states and
  should be explained separately.

## References

- [PR #12: fix(release): discover draft release by listing](https://github.com/meigma/sops-aws-sync/pull/12)
- Merge commit `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`
- [Immutable prerelease v0.1.1](https://github.com/meigma/sops-aws-sync/releases/tag/v0.1.1)
- [Release and attestation run](https://github.com/meigma/sops-aws-sync/actions/runs/29796898446)
- `.journal/007/MANUAL_E2E_TEST_PLAN.md` - manual acceptance procedure
- `.journal/007/EXECUTION_RESULT.md` - detailed evidence and cleanup record
- `.journal/006/SUMMARY.md` - release-ready baseline and deferred acceptance

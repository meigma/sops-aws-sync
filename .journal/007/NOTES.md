---
id: 007
title: Manual E2E release acceptance
started: 2026-07-20
---

## 2026-07-20 18:41 — Kickoff
Goal for the session: Start a new journal session; the substantive goal has not yet been stated.
Current state of the world: V1 implementation and hardening are merged on `master`; publication, repository configuration, and exact-release manual AWS acceptance remain open.
Plan: Wait for the actual request, then take the smallest useful implementation or investigation slice and refine from evidence.

## 2026-07-20 19:14 — Manual E2E acceptance plan
Goal: Draft and refine a user-only manual functional E2E test that provisions a temporary AWS sandbox with `whzbox`, creates a temporary private `meigma` consumer repository with `gh`, exercises happy paths and realistic failures through the released Action, and treats safety and cleanup as release gates.
Current evidence: The producer repository is private, has no published release, and currently exposes no private Action access to organization repositories; the authenticated GitHub user also cannot inspect the organization-wide Actions policy. A true release E2E therefore requires an authorized matching release or prerelease, temporary producer access, and a narrowly scoped producer-read token.
Outcome: Created `MANUAL_E2E_TEST_PLAN.md` with five stateful scenarios covering release trust and safe planning, ownership conflict plus idempotent creation, exact-commit and full lifecycle reconciliation, all-or-nothing invalid committed input with redaction checks, and the authorized empty-state/scope boundary. The plan includes isolated credential handling, exact run-to-commit binding, direct non-printing AWS evidence, pass/fail criteria, and mandatory cleanup.
Review: Refined the first draft after independent product, whzbox, adversarial, and execution-readiness reviews. The embedded workflow passes `actionlint`, every shell block parses with `bash -n`, and the document has no trailing whitespace.
Next: Wait for human review. Do not create the temporary repository, AWS sandbox, token, prerelease, or producer access change without an explicit execution request and the listed prerequisites.

## 2026-07-20 19:22 — Live execution authorized
Goal: Execute the approved acceptance plan to completion, including the prerelease candidate, temporary private consumer repository, isolated Whizlabs AWS sandbox, scenarios A–E, and mandatory cleanup.
Preflight: `master` and `origin/master` are clean and equal at `b67210d287faa642f7b4e1d7e73da438b5253abb`; the paired release metadata is consistently `0.1.1`; CI passed on that SHA. Producer private Action access was recorded as `none` and temporarily changed to `organization`.
Release evidence: Dispatched an exact-SHA Release Dry Run at https://github.com/meigma/sops-aws-sync/actions/runs/29795664004 and began waiting for it before creating the draft prerelease.
Credential gate: A narrow read-only producer credential must be created through GitHub's fine-grained token UI. The signed-in browser reached GitHub sudo mode, which requires user-held reauthentication; the broad `gh` OAuth token will not be uploaded as a fallback.
Next: Complete the exact-SHA rehearsal and prerelease, then resume the narrow-token path and disposable environment setup. Restore producer access to `none` on every exit path.

## 2026-07-20 19:40 — Acceptance blocked and cleanup complete
Result: **release blocker found before scenario A**. The exact-SHA Release Dry Run passed in 10m19s. A draft prerelease and exact `v0.1.1` tag then triggered the real Release workflow, but `Resolve Release` failed after all 30 attempts to run `gh release view v0.1.1`; the binary and attestation jobs were skipped. GitHub documents that the by-tag endpoint returns a published release, while this workflow explicitly waits for a draft, so this handoff cannot succeed as written.
Safety event: The first disposable sandbox printed its temporary credentials to the operator transcript because output suppression was omitted. It was treated as compromised and destroyed immediately. A fresh sandbox was provisioned with suppressed output and verified, but no credentials were uploaded and no consumer repository was created because the release gate had already failed.
Cleanup proof: Both sandbox accounts were destroyed; the isolated Whizlabs state directory was deleted; the draft release and exact test tag were deleted; producer private Action access was restored from `organization` to `none`; no producer token or consumer repository was created; `master` remains clean and equal to `origin/master`.
Next: Fix the draft-release handoff in `.github/workflows/release.yml` without weakening the trust gates, rerun the real release workflow to a fully built and attested prerelease, and then repeat this acceptance plan. Creating the required repository-scoped producer token will require GitHub sudo reauthentication by the user.

## 2026-07-20 19:46 — Draft-release handoff fix opened
Authorization: The user asked to resolve the release issue and resume testing.
Change: Created isolated branch `fix/release-draft-handoff` from clean `master`. The resolver now grants its job token `contents: write`, then polls the release-list endpoint and exact-matches a draft's tag instead of calling the published-only by-tag endpoint. The bounded wait and all downstream build, upload, and attestation gates are unchanged.
Verification: `go tool actionlint .github/workflows/release.yml` and the full `MOON_TOOLCHAIN_FORCE_GLOBALS=true mise exec -- moon run root:check` gate passed. Opened PR #12 at https://github.com/meigma/sops-aws-sync/pull/12 with head `e3862c8c9dce7060a4596646464ce866392d4736`.
Next: Verify hosted checks on that exact head, squash-merge through GitHub, fast-forward local `master`, then recreate the draft/tag and resume the live release plus scenarios A–E.

## 2026-07-20 19:52 — Handoff fixed; private attestation feature blocked
Correction: PR #12's exact reviewed head was `e3862c88c10824797af7b56329f565cd3684657a`; the prior abbreviated expansion in these notes was incorrect.
Merge: PR #12 passed CI, Release Dry Run, GitHub Pages build, and Kusari Inspector on that exact head, then squash-merged as `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`. Local `master` was fast-forwarded and the implementation worktree/branch were removed.
Live proof: Recreated the draft `v0.1.1` release and exact annotated tag. Release run https://github.com/meigma/sops-aws-sync/actions/runs/29796898446 resolved the draft in two seconds, built and smoke-tested all binaries, validated and uploaded all nine assets, and transferred `checksums.txt` into the isolated attestation job.
External blocker: `actions/attest` then failed to persist provenance with GitHub's exact error: `Feature not available for the meigma organization. To enable this feature, please upgrade the billing plan, or make this repository public.` The producer remains private, the release remains a draft prerelease, the exact tag and nine assets are retained, private Action access remains `none`, and no AWS sandbox, consumer repository, or producer token has been created in this resumed run.
Decision needed: Enable private-repository attestations through the organization plan, or explicitly authorize changing the producer repository's visibility. Do not publish the unattested release or bypass the Action's provenance requirement.

## 2026-07-20 20:29 — Acceptance passed and cleanup complete
Resolution: The user confirmed the producer is an OSS repository and changed it to public. The retained `v0.1.1` attestation job then passed, immutable releases were enabled, and the prerelease was published. `gh release verify` validates all nine assets and provenance at merge commit `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`.
Execution: Created temporary private consumer repository `meigma/sops-aws-sync-e2e-20260721030410` and Whizlabs AWS account `705991249149`. Scenarios A–E passed across release verification, non-mutating plans, ownership conflict, serialized repeat runs, pinned revisions, lifecycle changes, out-of-band repair, plaintext rejection and redaction, authorized empty state, and scope boundaries. See `EXECUTION_RESULT.md` for run URLs and exact evidence.
Observation: Removing every tracked source file also removes the Git directory. The Action safely treats this as a missing source root. An intentional empty snapshot needs a tracked placeholder such as `secrets/.gitkeep` before `allow-empty` can authorize scheduled deletion.
Cleanup: The user deleted the exact temporary consumer repository and GitHub confirms it is absent. The AWS sandbox was destroyed, a second destroy found nothing, isolated Whizlabs login state was removed, and the local clone and age key were moved to Trash. Producer `master` is clean and synchronized; the verified immutable prerelease remains intentionally published.
Outcome: **PASS — no release blocker remains after PR #12 and the public-repository attestation rerun.** Session remains open pending an explicit close request.

## 2026-07-20 21:55 — Close
Landed work: [PR #12](https://github.com/meigma/sops-aws-sync/pull/12) was reviewed, approved, and squash-merged as `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`; local `master` is clean, fast-forwarded, and has no remaining session implementation worktree or branch.
Release and acceptance: Immutable prerelease [`v0.1.1`](https://github.com/meigma/sops-aws-sync/releases/tag/v0.1.1) and all nine assets pass GitHub release verification. Manual scenarios A–E passed with direct AWS and hosted-log evidence; the detailed record is in `EXECUTION_RESULT.md`.
Cleanup: The temporary private consumer repository, AWS sandbox, isolated login state, local clone, and age key are absent. No temporary token or elevated GitHub scope remains.
Handoff: Session 007 closes complete. The release remains intentionally marked prerelease; stable promotion and the `.gitkeep` operator-documentation clarification are future work, not release blockers found by this acceptance run.

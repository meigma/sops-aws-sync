---
id: 003
title: Phase 2 durable AWS vertical slice
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002]
---

## Goal
Review session 001's authoritative V1 design and delivery plan, then implement, verify, and land Phase 2: the first durable committed-SOPS-to-AWS reconciliation slice.

## Outcome
The goal was met. [PR #8](https://github.com/meigma/sops-aws-sync/pull/8) was reviewed, approved, and squash-merged into `master` as `f3f995e79689e258992e2d423e1ac5a9c7aabb35`. The repository now has a production Go CLI that reads an exact committed Git snapshot, decrypts and canonicalizes SOPS JSON in process, classifies direct AWS Secrets Manager state, plans and applies create/update operations, handles ownership and ambiguous-write safety boundaries, and verifies convergence with stable reports and exit codes.

The final live acceptance test used whzbox account `175091678803` in `us-east-1` against reviewed head `77a8dcb1b4572364f65a985033cedad8bb507474`. `TestAWSSandboxCreateUpdateNoOpAndVerification` passed in 4.01 seconds and proved create → converged no-op → external-drift update → converged no-op against genuine AWS Secrets Manager. Cleanup scheduled deletion of the unique test secret under `/sops-aws-sync/phase2-sandbox`, and a direct metadata check confirmed a non-empty deletion timestamp.

## Key Decisions
- Build the complete immutable desired snapshot before loading AWS credentials -> invalid Git or SOPS input fails locally without contacting AWS.
- Classify ownership and safety from `DescribeSecret` metadata before reading `AWSCURRENT` -> foreign, service-owned, rotation-managed, replicated, and lifecycle-conflicting secrets never have payloads fetched.
- Treat an `AWSCURRENT` version without string or binary payload as invalid -> internally inconsistent AWS evidence fails closed instead of being overwritten.
- Rebuild the whole plan once after harmless precondition drift -> create-before-update phase ordering and report counts remain coherent; repeated churn terminates as verification failure.
- Allocate a fresh token per logical write and re-observe ambiguous mutations before a bounded same-token retry -> lost responses do not create duplicate versions or unsafe blind retries.
- Keep Phase 2 limited to direct desired-name create/update reconciliation -> restore, deletion, enumeration, and lifecycle policy remain Phase 3 work.
- Keep reports and logs non-sensitive by default -> stable schema, status, counts, timing, and normalized AWS error metadata are available without secret values or names.

## Changes
- `internal/domain` - added validated values, deterministic source-to-secret mapping, ownership/safety classification, pure planning, and precondition/ambiguous transition rules.
- `internal/application` - added immutable desired snapshots, bounded full-plan reconciliation, idempotency token handling, direct verification, safe reports, and application ports.
- `internal/adapters/gitrepo` and `internal/adapters/sopsdecrypt` - added exact-commit regular-file reads plus terminal in-process SOPS JSON decryption and canonicalization.
- `internal/adapters/secretsmanager` - added metadata-first direct observation, standard-chain AWS loading, safe error classification, and create/update mutations.
- `cmd/sops-aws-sync`, `internal/cli`, and `internal/config` - added the production Cobra/Viper CLI, stable exit contract, secret-safe diagnostics, report persistence, logging, and runtime composition.
- Repository/release/docs surfaces - rebranded the template to `sops-aws-sync`, reduced publishing to binary-only delivery, pinned workflow actions, and enforced lint, race, Godoc, build, and docs gates.
- Tests - added domain, adapter, application, CLI, subprocess, failure-boundary, ordering, redaction, and opt-in live AWS acceptance coverage.

## Open Threads
- Phase 3 remains intentionally deferred: direct enumeration, restoration, deletion scheduling, empty-state authorization, recovery-window handling, and related lifecycle policy.
- The whzbox sandbox used for acceptance was left to its normal one-hour expiry; the test secret is already in scheduled-deletion state.

## Lessons
- Metadata-first observation is both a confidentiality boundary and an availability improvement because ownership conflicts do not depend on payload-read permission.
- Per-resource operation substitution is insufficient when plan phases carry ordering guarantees; harmless drift must rebuild the complete plan.
- Functional acceptance against real AWS caught the final environmental proof obligation that unit and SDK-fake tests could not satisfy.

## References
- [PR #8: feat: add durable AWS reconciliation slice](https://github.com/meigma/sops-aws-sync/pull/8)
- Merge commit `f3f995e79689e258992e2d423e1ac5a9c7aabb35`
- Reviewed and AWS-tested head `77a8dcb1b4572364f65a985033cedad8bb507474`
- `.journal/001/DESIGN.md` - authoritative V1 design
- `.journal/001/PLAN.md` - five-phase delivery roadmap
- `.journal/002/SUMMARY.md` - accepted disposable Phase 1 proof

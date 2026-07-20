---
id: 004
title: Phase 3 complete CLI reconciliation
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002, 003]
---

## Goal

Review session 001's authoritative V1 design and delivery plan, then implement,
verify, review, and land Phase 3: the complete Go CLI lifecycle reconciliation
slice.

## Outcome

The goal was met. [PR #9](https://github.com/meigma/sops-aws-sync/pull/9)
was reviewed, approved, and squash-merged into `master` as
`f7d202f30fa0cd4fc1a681b5c41b0de445a6e4af`. The CLI now discovers the exact
managed scope, plans from the desired/discovered union, restores scheduled
secrets, schedules deletions with recovery, authorizes empty desired state,
applies lifecycle-safe ordering and preconditions, and verifies convergence.

The reviewed head `d0ecee141ac89d8e54ed5b587bdde6ff6b5e0d35` passed the full
Moon gate, the race suite, the opt-in genuine AWS lifecycle test, hosted CI,
GitHub Pages, and Kusari Inspector. Two P1 review findings were fixed before
merge: affected deletion targets are now directly observed even when discovery
omits them, and successfully applied restore bookkeeping survives harmless plan
rebuilds without permitting unrelated follow-up operations.

## Key Decisions

- Build plans from the union of desired names and paginated exact-scope discovery -> removed secrets can be scheduled for deletion without widening ownership scope.
- Require explicit authorization before reconciling an empty desired snapshot -> an accidental empty source cannot trigger mass lifecycle changes.
- Apply deletions last and permit only a bounded restore follow-up -> destructive work remains ordered after constructive convergence, while restored secrets can receive their intended update.
- Retain affected targets and successful restores independently of rebuilt plans -> stale discovery cannot create false convergence and harmless rebuilds cannot erase safety constraints.
- Give every live AWS test run a unique prefix and ownership scope -> scheduled remnants from earlier runs cannot pollute later acceptance evidence.

## Changes

- `internal/domain` - added exact-scope lifecycle observations and deterministic create, restore, update, no-op, and scheduled-deletion planning.
- `internal/adapters/secretsmanager` - added paginated discovery, restore, scheduled deletion, and lifecycle-aware observation through AWS Secrets Manager.
- `internal/application/reconcile.go` - added desired/discovered-union reconciliation, empty-state authorization, deletion-last application, bounded rebuild and restore cycles, affected-target verification, and safe partial-failure behavior.
- `internal/application/reconcile_test.go` - added lifecycle, ordering, rebuild, stale-discovery, restore-bookkeeping, and verification regressions.
- `internal/application/sandbox_test.go` - expanded the opt-in genuine AWS proof through create, no-op, drift update, discovery, deletion, empty-state convergence, restore, follow-up update, and final convergence.
- `cmd/sops-aws-sync/main.go` - wired the complete lifecycle reconciliation behavior into the production CLI.

## Open Threads

- Phase 4 is next: implement the thin Node 24 TypeScript GitHub Action around the completed Go CLI, as defined by session 001's design and plan.
- Phase 5 remains the V1 hardening, documentation, release, and exact-merged-SHA acceptance phase.
- The unique AWS sandbox secret used by the acceptance test was left scheduled for deletion; the whzbox sandbox remains subject to its normal expiry.

## Lessons

- `ListSecrets` is discovery evidence, not sufficient deletion verification; every affected lifecycle target must also be observed directly.
- Safety state must describe operations that actually succeeded, not merely the latest rebuilt plan, because rebuilds may legitimately omit already-applied restores.
- Functional acceptance against genuine AWS exposed shared scheduled-deletion residue that deterministic unit fakes could not reveal.

## References

- [PR #9: feat: complete CLI lifecycle reconciliation](https://github.com/meigma/sops-aws-sync/pull/9)
- Merge commit `f7d202f30fa0cd4fc1a681b5c41b0de445a6e4af`
- Reviewed and AWS-tested head `d0ecee141ac89d8e54ed5b587bdde6ff6b5e0d35`
- `.journal/001/DESIGN.md` - authoritative V1 design
- `.journal/001/PLAN.md` - five-phase delivery roadmap
- `.journal/003/SUMMARY.md` - Phase 2 durable reconciliation baseline

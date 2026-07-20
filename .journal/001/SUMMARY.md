---
id: 001
title: CLI and GitHub Action design and V1 plan
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: []
---

## Goal

Define a reviewable V1 design and companion delivery plan for `sops-aws-sync`:
a Go CLI and TypeScript GitHub Action that reconcile committed SOPS-encrypted
JSON documents with AWS Secrets Manager.

## Outcome

The goal was met. The session produced a refined, security-reviewed design and
a five-phase implementation plan. The repository and journal were bootstrapped,
but product implementation did not begin. No implementation PR was opened or
merged, and the default `master` branch remained at the template's initial
commit.

## Artifact Catalog

- [`DESIGN.md`](./DESIGN.md) is the sole authority for V1 behavior,
  architecture, security, configuration, operations, testing, incremental
  proofs, and acceptance. Future implementation sessions must read it before
  making product changes.
- [`PLAN.md`](./PLAN.md) is the companion delivery roadmap. It sequences the
  approved design into five consecutive, single-PR phases without adding or
  changing design requirements. Future sessions should use it to identify the
  next phase, then return to `DESIGN.md` for the governing contract.
- [`NOTES.md`](./NOTES.md) is the chronological record of product direction,
  review findings, refinements, and session checkpoints. Use it when the reason
  behind a design or phase-boundary correction matters.

When these artifacts appear to conflict, `DESIGN.md` prevails. Stop and request
a design decision rather than resolving an ambiguity in code or silently
rewriting the plan.

## Key Decisions

- Desired state comes from regular `.sops.json` blobs at an exact Git commit;
  correctness does not depend on the working tree or a prior successful run.
- The reconciler uses a pure, strongly typed domain over desired and observed
  snapshots. Git, SOPS, AWS, the CLI, and the Action remain adapters around
  that business logic.
- Managed-secret ownership and lifecycle are explicit and fail closed. The
  design covers creation, updates, no-ops, restoration, scheduled deletion,
  preconditions, ambiguous outcomes, and verification without destructive
  force deletion.
- The Go implementation uses go-git, SOPS Go bindings, AWS SDK for Go v2, and
  Viper within clean hexagonal boundaries, with complete Godoc and
  non-sensitive production logging.
- The Node 24 TypeScript Action is a thin, strongly typed, shell-free process
  adapter around the Go CLI. Binary downloads and cache hits require checksum
  and GitHub-attestation verification.
- Delivery follows five ordered PR phases: disposable committed-source proof,
  durable AWS vertical slice, complete CLI reconciliation, TypeScript Action,
  and V1 hardening plus exact-merged-SHA acceptance evidence.

The exact language and full constraints for these decisions remain in
`DESIGN.md`.

## Changes

- `.journal/001/DESIGN.md` — added and iteratively refined the complete CLI and
  Action design; committed in `b1c3736`.
- `.journal/001/PLAN.md` — added and iteratively refined the design-bound V1
  implementation plan; committed in `34708c2`.
- `.journal/001/NOTES.md` — recorded the project direction, review corrections,
  and final handoff state.
- `.journal/TECH_NOTES.md` — points future sessions to the authoritative design
  and companion plan.

## Open Threads

- Implementation has not started. Begin only after explicit user approval and
  start with Phase 1 in `PLAN.md`, whose disposable draft PR closes without
  merge.
- Phases 2 through 5 remain unimplemented. Each is one mergeable PR and starts
  only after the preceding phase satisfies its success criteria.
- Any implementation evidence that conflicts with or exposes ambiguity in the
  design requires user direction before work continues.

## Lessons

- A vertical slice that can mutate AWS must include every design-defined
  desired-name safety classification before exposing writes, even when later
  lifecycle operations remain deferred.
- Release and protected-workflow evidence that requires the default branch must
  run from the exact merged Phase 5 commit; its pre-merge gates remain distinct.

## References

- [`DESIGN.md`](./DESIGN.md)
- [`PLAN.md`](./PLAN.md)
- [`NOTES.md`](./NOTES.md)
- Repository: <https://github.com/meigma/sops-aws-sync>
- Template source: <https://github.com/meigma/template-go>

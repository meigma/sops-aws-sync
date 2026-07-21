---
id: 008
title: Diátaxis operator documentation
date: 2026-07-20
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 006, 007]
---

## Goal

Treat the existing `docs/` tree as defunct, comprehensively review the codebase
with small multi-agent workflows (Opus 4.8 / Sonnet 5 agents only), propose a
Diátaxis-adherent operator documentation structure, and — after the user
approved the proposal with full execution autonomy — author, verify, and land
the complete replacement documentation set.

## Outcome

The goal was met. Two workflows produced an evidence-grounded proposal: seven
subsystem readers built a 223-fact operator map of the system
(`.journal/008/SYSTEM_MAP.json`), then three independent designers, a
three-judge panel, and a synthesizer produced the approved 13-document
structure (`.journal/008/DOC_STRUCTURE_PROPOSAL.md`). Execution authored all
14 files (structure plus landing) with per-document adversarial code
fact-checking, Diátaxis purity review, fixes, and a whole-set consistency
pass.

Mid-execution, master merged PR #13 (YAML SOPS document support), which
conflicted with the branch and invalidated JSON-only claims. The branch was
rebased and a third workflow extracted 31 verified fact deltas and revised
nine documents. [PR #14](https://github.com/meigma/sops-aws-sync/pull/14)
was squash-merged as `38549eb5bb9b3beaab687ff0ed974cee9f2d103f` with CI, the
binary release dry run, Pages, and Kusari green before and after merge. The
published site at <https://meigma.github.io/sops-aws-sync/> was verified to
serve the new set, including the YAML coverage.

## Key Decisions

- Four-explanation spine with `ownership-and-scope` standalone -> the silent
  re-scoping trap is the top operator hazard and earns a dedicated read.
- Three-reference inputs→rules→outputs triad (configuration / reconciliation /
  results) -> every enumerable fact catalogued exactly once; explanations
  narrate concepts and never reproduce tables.
- One tutorial only, lifecycle-shaped and JSON-only -> Diátaxis demands one
  option-free path; CI deployment is a how-to, not a lesson.
- Per-document deliberate-exclusion lists kept in the published docs -> they
  are the mechanism that keeps type boundaries enforced during grooming.
- Fact-checkers ranked below code -> fixers re-verified every finding against
  the worktree before applying; several reviewer and spec claims were rejected
  on code evidence, and the proposal spec itself was corrected twice.
- `secretsmanager:TagResource` resolved decisively in `grant-aws-access` ->
  the tool never calls it standalone, but AWS requires the permission for
  tagged `CreateSecret`.
- Encoding-only rename guidance kept concrete -> a JSON→YAML re-encode at the
  same stem preserves name, source identity, ownership, and value (logical
  source identity), while stem/directory moves remain delete-plus-create.

## Changes

- `docs/docs/` - replaced four legacy pages with the 14-document Diátaxis set
  (landing, 1 tutorial, 5 how-tos, 4 explanations, 3 references).
- `docs/mkdocs.yml` - new nav for the four-section structure.
- `README.md` - documentation section now routes by operator moment; the
  release-verification link points at the new how-to.
- `.journal/008/DOC_STRUCTURE_PROPOSAL.md` - the approved structure authority,
  including two code-verified spec corrections made during execution.
- `.journal/008/SYSTEM_MAP.json` - the seven-reader evidence base with
  file:line citations.

## Open Threads

- The live docs describe master behavior including PR #13's YAML support, but
  the latest release is still `v0.1.1`, which predates it; the next release
  closes that gap (release authorization remains open from session 007).
- Tutorial Step 6 depends on the AWS console UI for the drift edit and needs
  active maintenance when that UI changes.
- Release Please and GitHub Pages now succeed on master, so session 006's
  credential/Pages open threads appear externally resolved; nobody has
  re-verified the Release Please configuration end to end with a real release.

## Lessons

- A docs branch racing an active codebase must re-verify facts after every
  rebase: PR #13 landed mid-execution and silently invalidated a load-bearing
  "JSON only" claim across nine documents.
- golangci-lint's shared cache replays stale results with dead absolute paths
  after a sibling worktree is deleted; `golangci-lint cache clean` fixes it —
  do not chase the phantom findings.
- GitHub refuses to build a PR's merge ref when it conflicts with base, so
  pull_request workflows silently never start; "no checks reported" on a PR is
  a conflict symptom, not a CI outage.
- Adversarial verification earns its cost: it caught a dead Action failure
  message, a wrong attestation-subject model, misplaced log-ordinal teaching,
  and two errors in the approved spec itself.

## References

- [PR #14: docs: replace operator documentation with Diátaxis structure](https://github.com/meigma/sops-aws-sync/pull/14)
- Merge commit `38549eb5bb9b3beaab687ff0ed974cee9f2d103f`
- [Published documentation](https://meigma.github.io/sops-aws-sync/)
- `.journal/008/DOC_STRUCTURE_PROPOSAL.md` - structure and boundary authority
- `.journal/008/SYSTEM_MAP.json` - code-cited operator fact base
- `.journal/008/NOTES.md` - execution log including workflow run IDs
- [PR #13: feat: support YAML SOPS documents](https://github.com/meigma/sops-aws-sync/pull/13) - the mid-flight behavior change
- `.journal/007/SUMMARY.md` - release baseline this session documents

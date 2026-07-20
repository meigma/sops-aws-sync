# sops-aws-sync V1 Implementation Plan

- Status: ready for review
- Authority: [`DESIGN.md`](./DESIGN.md)
- Scope: sequence the approved design into five bounded pull requests

## 1. Authority and change control

`DESIGN.md` is the sole authority for product behavior, architecture, security,
configuration, operations, testing, and V1 acceptance. This plan only orders
that design into reviewable implementation phases.

The following rules apply throughout:

- An implementation PR must not edit `DESIGN.md`.
- This plan must not add, remove, weaken, extend, or reinterpret a design
  requirement.
- When this plan summarizes a requirement, the exact design language prevails.
- If implementation evidence conflicts with the design or exposes an ambiguity,
  stop the phase and ask the user. Do not resolve the issue in code or in this
  plan.
- For reversible details not fixed by the design, follow an existing repository
  convention or choose the smallest implementation that proves the phase. Do
  not record alternatives or new design decisions in this plan.

## 2. Delivery rules

The phases follow Design Section 19 and execute in order. Each phase uses one PR
from opening through review and disposition. Phase 1 is a disposable draft PR
that closes without merging; Phases 2 through 5 are mergeable PRs. A phase
starts only after the preceding PR has met its success criteria and has been
closed or squash-merged as stated.

Every phase PR must:

- cite the design sections it implements;
- stay within its stated outcome and defer later-phase work;
- preserve the dependency boundaries in Design Sections 5 and 9;
- include the tests and evidence required by its success criteria;
- keep the repository gates green, including `moon run root:check`;
- use a Conventional Commit PR title; and
- be reviewed before it is closed or squash-merged through GitHub.

Keep all iteration and review fixes on that phase's PR. Do not combine,
overlap, split, supersede, or replace phase PRs. Any exception requires a
user-approved plan revision before work resumes. Do not compensate by changing
the design.

A later phase can fix an already-merged defect only when the fix is required to
prove that phase's design-defined outcome and remains inside its single PR.
Defer unrelated defects or request a user-approved plan revision.

## 3. Phase overview

| Phase | PR disposition | Observable outcome |
|---|---|---|
| 1. Committed-source planning proof | Draft PR, closed without merge | Proves one committed SOPS document becomes one pure `create` decision without AWS access. |
| 2. Durable AWS vertical slice | Merged PR | Delivers the production-shaped CLI path for create, update, no-op, and direct verification. |
| 3. Complete CLI reconciliation | Merged PR | Completes ownership, lifecycle, failure, interruption, and convergence behavior. |
| 4. TypeScript Action | Merged PR | Wraps the proven CLI in the design-defined, strongly typed GitHub Action. |
| 5. V1 hardening and acceptance | Merged PR, then exact-SHA evidence | Completes release integration, protected workflow proof, documentation, and all V1 acceptance criteria. |

## 4. Phase 1 — Committed-source planning proof

**Design references:** Section 5's dependency boundary; Sections 6.1 and 6.2;
Section 7; Section 8.1's `create` decision; Section 9's dependency rule;
Section 12; the relevant tests in Sections 17.1 and 17.2; and Section 19 item 1.

### Outcome

Prove the highest-risk source boundary with disposable code: one regular
`.sops.json` blob from an exact Git commit is decrypted through the SOPS Go
binding, validated and canonicalized, mapped to one secret identity, and planned
as `create` against absent observed state. The proof performs no AWS operation.

### Work

Keep the spike limited to the source-to-domain path. Use go-git rather than the
working tree or `git` executable, use `decrypt.DataWithFormat`, and keep the
planner independent of Git and SOPS. Record only reproducible, non-sensitive
evidence. Do not build durable CLI, AWS, Action, or release infrastructure.

### Success criteria

Phase 1 is complete only when:

- changing the working tree does not change the blob read from the selected
  commit;
- the SOPS binding verifies and decrypts a hermetic JSON fixture in process;
- the JSON contract and JCS canonicalization produce the expected canonical
  top-level object;
- path mapping and the pure planner deterministically return one `create`;
- tests prove no AWS client or mutation is involved and no sensitive fixture
  data reaches output; and
- existing repository gates remain green and the draft PR records the proof.

After review, close the draft PR without merging the spike. A design conflict
blocks Phase 2.

## 5. Phase 2 — Durable AWS vertical slice

**Design references:** Sections 4 and 5; Sections 6.1 and 6.2 plus the create and
same-name rules in Section 6.3; Section 7; Section 8's create, update,
unchanged, precondition, and plan rules; Sections 9 and 10; Sections 11.1
through 11.4 for create/update; Sections 12 through 14; Section 16; Sections
17.1 and 17.2; Section 18's repository identity and binary-only shape; and
Section 19 item 2.

### Outcome

Deliver the smallest production-shaped Go CLI that reconciles a committed SOPS
document through create, update, no-op, and direct post-apply verification
against one sandbox AWS secret.

### Work

Apply the repository identity and binary-only template changes required by
Design Section 18. Establish the design-defined domain, application, adapter,
CLI, and configuration boundaries. Implement the
`plan`, `sync`, and `version` contracts; exact commit loading; SOPS decryption;
JSON validation; typed configuration; safe logs and reports; standard-chain AWS
loading; direct desired-name observation; and the create/update/no-op path. The
public CLI and configuration surface from Design Section 13 is complete in this
phase; Phase 3 completes the remaining lifecycle-specific outcomes.

Create secrets with the reserved ownership tags. Implement the pure classifier
for every state observed directly at a desired name, including ownership,
source identity, lifecycle, rotation, service ownership, replication, and
staging constraints. Only a plan containing create, update, or no-op is
supported in this phase: only `create` and `update` operations are applied,
`unchanged` remains a no-op, and every other state remains non-mutating until
its design-defined operation arrives in Phase 3. Implement typed preconditions,
bounded SDK behavior, ambiguous-outcome re-observation, fresh logical-write
tokens, and terminal SOPS cancellation for create and update before exposing
`sync`. Leave scope-wide discovery, restoration, and deletion for Phase 3.

### Success criteria

Phase 2 is complete only when:

- all active repository, module, binary, task, and release identity is
  `sops-aws-sync`, with the binary-only release shape preserved;
- `plan`, `sync`, and `version` expose and validate the complete design-defined
  command and configuration surface, with streams, reports, and exit behavior
  correct for every outcome implemented in this phase;
- invalid desired state and every design-defined desired-name ownership or
  constraint conflict cause no AWS mutation;
- a missing sandbox secret is created with the canonical `SecretString` and
  exact reserved tags, changed content creates one update, and identical content
  creates no new version;
- external drift is repaired even when the desired Git commit is unchanged,
  proving that a new logical write does not reuse a stale request token;
- cancellation, transient failure, and an ambiguous create or update never
  trigger a blind write retry and produce a safe, resumable result;
- direct re-observation and pure re-planning end with zero executable operations
  and zero conflicts;
- default logs and reports pass sentinel non-disclosure tests;
- all Go declarations satisfy the design's Godoc policy; and
- unit, adapter, CLI, sandbox, and repository gates pass.

## 6. Phase 3 — Complete CLI reconciliation

**Design references:** Section 6.2's empty-state rule; Section 6.3; Sections 8,
10, 11.2 through 11.4, 13.2, and 14; Sections 17.1 and 17.2; Section 19 item 3;
and every CLI-related criterion in Section 20.

### Outcome

Complete the Go CLI's V1 reconciliation, ownership, lifecycle, interruption,
failure, and observed-snapshot convergence contracts.

### Work

Add scope-wide paginated discovery, extend pure ownership classification from
desired names to all discovered scope members, add multi-secret planning,
restoration, scheduled deletion, empty-state protection,
restore/delete preconditions and ambiguous-write transitions, multi-operation
partial-failure recovery, and final verification. Complete the remaining
domain matrix, fuzz invariants, lifecycle exit outcomes, and redaction cases
tied to those behaviors.

### Success criteria

Phase 3 is complete only when:

- the pure domain tests cover the full decision matrix, deterministic ordering,
  precondition transitions, conflicts, and planner invariants without
  infrastructure dependencies;
- executable operations are ordered restore, create, update, then scheduled
  deletion, with deletions always last;
- scope membership requires exact `managed-by` and `scope` tags plus a valid
  source digest; a desired same-name secret additionally requires the matching
  source identity;
- foreign, malformed, rotation-managed, service-owned, replica, and staging
  conflicts fail before mutation when observable;
- removed managed sources schedule deletion with the configured recovery window
  and never force deletion;
- reintroduced scheduled secrets restore and use only the permitted follow-up
  cycle before verification;
- cancellation, throttling, service failure, lost responses, ambiguous writes,
  partial apply, and inconclusive verification produce the design-defined safe
  results and allow a later run to converge;
- an empty desired set requires explicit authorization only when it would add
  scheduled deletions;
- default and debug output pass all secret, identifier, provider, and raw-error
  non-disclosure tests; and
- unit, fuzz-seed, adapter, functional sandbox, and repository gates pass.

At this point, the Go CLI V1 contract is complete.

## 7. Phase 4 — TypeScript GitHub Action

**Design references:** Section 4's Action baseline; Section 5's process
boundary; Section 15; Section 16's TypeScript policy; Sections 17.3 and 17.4;
Section 19 item 4; and the Action-related criteria in Section 20.

### Outcome

Deliver the design-defined Node 24 Action as a thin, strongly typed,
shell-free process adapter around the proven CLI.

### Work

Import the pinned `actions/typescript-action` snapshot and record its provenance.
Implement total input parsing, exact argument construction, safe CLI execution,
redacted report handling, outputs, summaries, exact-version installation,
checksum and GitHub-attestation verification, verified caching, and explicit
exit handling. Add the Action's Moon, npm, bundle, and `check-dist` gates.

Keep all Git, SOPS, AWS, reconciliation, retry, and ownership logic in Go.

### Success criteria

Phase 4 is complete only when:

- the pinned Node 24, ESM, NodeNext, strict-TypeScript template baseline and its
  recorded provenance are intact;
- every Action input is totally parsed into immutable typed configuration and
  maps only to the design-defined CLI arguments;
- the Action invokes an absolute CLI path in `GITHUB_WORKSPACE` without a shell,
  raw argument echo, or arbitrary argument input;
- downloads and cache hits fail closed unless checksum and GitHub attestation
  verification both enforce the complete identity, ref, digest, issuer,
  hosted-runner, compatible-`gh`, exact-version, no-fallback, and private-access
  policy in Design Section 15.4;
- plan drift behavior, CLI exit propagation, safe outputs, summaries, temporary
  report handling, and token masking match Design Section 15;
- a dependency check confirms the Action imports no AWS SDK, tests cover only
  the five responsibilities in Design Section 15.2, and a functional test
  invokes the real Go binary;
- every workflow introduced or touched by the template import pins external
  Actions to full commit SHAs; and
- format, lint, typecheck, Jest, coverage, bundle, `check-dist`, and repository
  gates pass.

## 8. Phase 5 — V1 hardening and acceptance

**Design references:** Section 3; Section 15.6; Section 17.4; Sections 18 and
19 item 5; and Section 20.

### Outcome

Complete repository integration and record end-to-end evidence that the CLI and
Action together satisfy every V1 acceptance criterion in `DESIGN.md`.

### Work

Finish the unified binary-and-Action release path, provenance, SBOM, CI,
dependency automation, CODEOWNERS coverage, protected branch rules, and
operator documentation already required by the design. Add the protected,
serialized sandbox workflow that supplies AWS authentication before invoking
the Action. Prepare all evidence that can run on the PR, then use the exact
squash-merged SHA for release and protected workflow evidence that requires the
workflow to exist on the default branch.

### Success criteria

Before the Phase 5 PR merges:

- Moon's root gate covers the complete Go, Action, bundle-drift, and
  documentation checks defined in Design Section 17.4;
- the pre-merge release proof builds the supported Linux and macOS binaries,
  checksums, SBOMs, and matching committed Action bundle, and validates the
  provenance workflow without publishing a release;
- repository-owned workflows use full-SHA action pins; CODEOWNERS covers
  `action.yml`, `action/**`, and privileged workflows; and branch protection
  requires code-owner approval and the repository's required checks;
- operator documentation states the design-defined configuration, AWS/KMS/IAM,
  authentication, interruption, consistency, logging, and workflow-trust
  contracts without adding guarantees;
- every Design Section 20 criterion that can be established before merge has
  recorded evidence, and every V1 non-goal in Design Section 3 remains excluded;
  and
- the complete repository gate passes on the reviewed head.

After the PR is squash-merged and the user authorizes release publication,
Phase 5 completes only when:

- one immutable release from the exact merged commit contains the supported
  binaries, checksums, SBOMs, matching Action bundle, and verified GitHub
  provenance attestations;
- the protected sandbox workflow runs from that commit and proves create,
  update, no-op, restore, scheduled deletion, safe logs, and verified
  observed-snapshot convergence through the Action using caller-owned AWS
  authentication and one-writer concurrency; and
- every remaining Design Section 20 criterion has recorded evidence.

Post-merge evidence does not create a second phase PR. If it exposes a defect
that requires code changes, stop and ask the user for a plan revision. When all
post-merge criteria pass, V1 is implemented.

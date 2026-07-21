# Results and exit codes reference

This document catalogs every result the tool emits: the process exit
codes of the `sops-aws-sync` CLI, the run statuses and verification
values recorded in the machine report, the report file schema, and the
failure messages the GitHub Action publishes. The process exit code and
the report `status` are two views of one outcome and are required to
agree. Every value below is stable within a release and is the contract
on which CI logic depends.

For what to *do* about a given result, see
[How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md).
The taxonomy of conflict classes behind exit code `4` is cataloged in the
[Reconciliation reference](reconciliation.md). Flag, environment, and
Action input/output definitions and the stream contract are in the
[Configuration reference](configuration.md).

## Exit codes

The CLI terminates with one of the following codes. `plan` and `sync`
share the same set; `version` and the bare root command exit `0` on
success.

| Code | Condition |
|------|-----------|
| `0` | Success. `sync` converged; `plan` completed (converged or drift). |
| `2` | `plan` detected executable drift. Emitted only when `--detailed-exit-code` is set (the Action adds it in plan mode with `fail-on-drift`). |
| `3` | Invalid configuration or desired state; also forced when the `--report-file` write fails. |
| `4` | An ownership or observed-state conflict blocked the plan. |
| `5` | Apply failed or observation failed; also the fallback for an unrecognized application outcome. |
| `6` | Verification failed or was inconclusive. |
| `130` | The run was interrupted. |

Notes:

- Without `--detailed-exit-code`, `plan` exits `0` even when the plan
  reports `status: drift`. Exit code `2` requires `--detailed-exit-code`
  *and* at least one executable operation (create, update, restore, or
  scheduled deletion) in the plan.
- Exit `3` is produced before the outcome code whenever writing the
  report file fails, so a report-write failure masks the underlying
  outcome.
- Exit `130` covers `SIGINT`/`SIGTERM` and a `--run-timeout` deadline;
  interruption during setup and an exceeded run timeout are
  indistinguishable by exit code.
- Any application outcome the CLI does not recognize resolves to exit
  `5`.

## Run statuses

The report `status` field carries one of the following values. `drift`
is produced only by `plan`; every other value can be produced by `sync`,
and the setup-phase values (`invalid`, `conflict`, `observation-failed`,
`interrupted`) can also be produced by `plan`.

| Status | Produced by | Description |
|--------|-------------|-------------|
| `converged` | `plan`, `sync` | Zero executable operations and zero conflicts; the scope matches the desired state. |
| `drift` | `plan` | The plan contains at least one executable operation. |
| `conflict` | `plan`, `sync` | At least one scope member is unowned or unsafe to touch; the whole plan is blocked before any mutation. |
| `invalid` | `plan`, `sync` | The configuration or desired state could not be resolved, or an empty desired snapshot would schedule deletions without `--allow-empty`. |
| `apply-failed` | `sync` | A mutation failed, or an ambiguous restore or scheduled deletion could not be resolved. |
| `observation-failed` | `plan`, `sync` | Discovery or direct observation of the desired-and-scope union failed before planning. |
| `interrupted` | `plan`, `sync` | The context was cancelled or its deadline elapsed during preflight, apply, or verification. |
| `verification-failed` | `sync` | Post-apply verification did not converge, or the benign-rebuild budget was exhausted. |
| `verification-inconclusive` | `sync` | Writes likely landed but the scope did not stabilize within `--verification-timeout`. |

## Verification values

The report `verification` field records the outcome of the post-apply
verification phase. `plan` never verifies and always reports `not-run`.

| Value | Description |
|-------|-------------|
| `not-run` | Verification did not run: `plan`, or a `sync` that failed before or during apply. |
| `converged` | The scope was independently re-observed and matched the desired state. |
| `failed` | The scope did not converge, or the benign-rebuild budget was exhausted. |
| `inconclusive` | The scope did not stabilize within `--verification-timeout`; distinct from an interrupted parent context. |

## The outcome contract

The process exit code, the report `status`, the report `verification`
value, and the Action failure message are a single agreed contract. Each
exit code pairs only with the statuses and verification values shown
below.

| Exit | Report status | Verification | Action failure message |
|------|---------------|--------------|------------------------|
| `0` | `converged` (sync); `converged` or `drift` (plan) | `converged` (sync); `not-run` (plan) | — (step succeeds) |
| `2` | `drift` | `not-run` | `sops-aws-sync plan detected drift` |
| `3` | `invalid` | `not-run` | `sops-aws-sync rejected the Action configuration or desired state` |
| `4` | `conflict` | `not-run` | `sops-aws-sync found an ownership or observed-state conflict` |
| `5` | `apply-failed`, `observation-failed` | `not-run` | `sops-aws-sync apply did not complete safely` |
| `6` | `verification-failed`, `verification-inconclusive` | `failed`, `inconclusive` | `sops-aws-sync verification did not converge` |
| `130` | `interrupted` | `not-run` | `sops-aws-sync was interrupted` |

The Action cross-checks the numeric exit code against the report
`status` before publishing any output. A pairing outside the table —
including any unrecognized nonzero exit code, which by definition has no
row above — fails the step with the message `CLI exit status and
redacted report disagree` and publishes zero outputs; this is the signal
that a pinned CLI version and the Action version have diverging
exit-or-status semantics.

Exit code `2` marks the Action step failed with the drift message; this
is the intended behavior of the `fail-on-drift` gate.

## Action failure messages

The Action publishes a fixed, non-sensitive message for each terminal
state through `setFailed`. The CLI itself emits no such message — it
emits an exit code and writes the report. The messages are:

| Trigger | Message |
|---------|---------|
| Exit `2` | `sops-aws-sync plan detected drift` |
| Exit `3` | `sops-aws-sync rejected the Action configuration or desired state` |
| Exit `4` | `sops-aws-sync found an ownership or observed-state conflict` |
| Exit `5` | `sops-aws-sync apply did not complete safely` |
| Exit `6` | `sops-aws-sync verification did not converge` |
| Exit `130` | `sops-aws-sync was interrupted` |
| Exit code and report status disagree, or any unrecognized nonzero exit | `CLI exit status and redacted report disagree` |
| An unexpected internal error | `sops-aws-sync Action execution failed` |

The Action defines one further fallback message, `sops-aws-sync returned
an unsupported exit status`, for a nonzero code outside the set above.
The exit-or-status cross-check runs first and rejects any such code with
the disagreement message, so this fallback never reaches step output.

Independently of the exit code, if the CLI writes anything to standard
error the Action emits the fixed warning `sops-aws-sync wrote a
diagnostic to stderr`. The standard-error content itself is never
logged.

## Report schema

The CLI writes a machine report to the `--report-file` path; the Action
writes it to a temporary path and exposes it as the `redacted-report-path`
output. The report is a JSON object identified by the schema
`sops-aws-sync/report/v1`.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always the literal `sops-aws-sync/report/v1`. |
| `tool_version` | string | The CLI release version embedded at build time; non-empty. |
| `git_revision` | string | The resolved 40-character commit SHA; may be empty when a run fails before the revision resolves. |
| `status` | string | One of the run statuses above. |
| `counts` | object | The six decision totals below. |
| `verification` | string | One of the verification values above. |
| `duration_ms` | integer | Total run duration in milliseconds. |

### Counts

The `counts` object contains exactly these six non-negative integers.

| Field | Description |
|-------|-------------|
| `create` | Create operations. |
| `update` | Update operations. |
| `restore` | Restore operations. |
| `schedule_delete` | Scheduled-deletion operations. |
| `unchanged` | No-op decisions. |
| `conflicts` | Blocking conflicts. |

### Guarantees

- Written atomically (temporary file plus rename) with file mode `0600`.
- Written even when the run fails during setup or preflight; the report
  always reflects the final `status`.
- A report-file write failure forces the CLI to exit `3`.
- The Action validates the report before reading it: the key set must be
  exactly the seven fields above (extra or missing keys are rejected),
  `schema_version` must equal the literal, `counts` must contain exactly
  the six integer fields, `tool_version`, `status`, and `verification`
  must be non-empty, string fields must be at most 1024 characters and
  free of control characters, `git_revision` may be empty, and the file
  must be a regular JSON file of 1 to 65,536 bytes.

## Log fields

Structured logs are written to standard output. Each record carries only
non-sensitive fields drawn from this vocabulary:

- `phase` — the run phase (`plan`, `apply`, `verify`, `aws`, `report`,
  or `complete`).
- `status` — the phase or run status.
- `operation` — the operation kind, or the AWS operation name on an AWS
  error record.
- `count` — the number of planned operations.
- `duration_ms` — elapsed milliseconds.
- `resource` — the target resource, shown as an ordinal `resource-NNNN`
  (zero-padded to four digits, for example `resource-0001`) unless
  `--show-resource-names` is set.
- `aws_error_code`, `fault`, `request_id` — normalized AWS failure
  metadata; raw AWS error text never appears.

The full stdout, stderr, and report stream contract is in the
[Configuration reference](configuration.md).

## Not cataloged here

| Topic | Owner |
|-------|-------|
| What to do about a given exit code or status | [How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md) |
| The conflict-class taxonomy behind exit `4` | [Reconciliation reference](reconciliation.md) |
| Why verification can be inconclusive, why ordering and idempotency behave as they do | [About consistency, verification, and recovery](../explanation/consistency-and-recovery.md), [About the reconciliation model](../explanation/reconciliation-model.md) |
| Flag, environment, and Action input/output definitions | [Configuration reference](configuration.md) |
| Why the report and logs are redacted | [About the security and trust model](../explanation/security-and-trust.md) |

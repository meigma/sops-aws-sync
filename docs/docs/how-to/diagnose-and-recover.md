# How to diagnose and recover from a failed run

Turn a non-zero exit code or an Action failure message into a concrete next
action. The tool withholds raw error detail on purpose, so this guide keys the
remedy to the one signal you always have: the exit code and its matching report
status.

## First, orient

Collect three signals before you act:

- The process **exit code** (`3`, `4`, `5`, `6`, or `130` on a failed run).
- The report `status` and `verification` value from the `--report-file` JSON.
- For the Action, the fixed **failure message** on the failed step.

For what each code, status, and verification value *means*, see the
[results reference](../reference/results.md). This guide supplies what to *do*.

Read the JSON logs on stdout and the report file. Raw stderr is suppressed: the
CLI's stderr is never forwarded, and the Action emits only the fixed warning
`sops-aws-sync wrote a diagnostic to stderr` when the CLI writes anything there.
Do not expect stack traces or AWS error text in the logs — that redaction is
deliberate ([why](../explanation/security-and-trust.md)).

!!! note "Re-running is the general recovery path"
    Completed AWS calls are never rolled back. On any partial failure the tool
    stops before deletions, exits non-zero, and a later run resumes from
    observed state — sync is resumable-forward, not transactional
    ([why](../explanation/consistency-and-recovery.md)). A converged scope is a
    pure no-op and create/update are idempotent, so re-running after a fix is
    safe.

Then jump to your exit code below. After any fix, confirm recovery with the
final step.

## Exit 3 — invalid configuration or desired state

Action message: `sops-aws-sync rejected the Action configuration or desired
state`.

The run was rejected before or during planning. Common causes and what to do:

1. **A rejected flag or an unknown configuration key.** Configuration
   validation fails closed on any unrecognized or misspelled key or a type
   mismatch. Fix the offending value; see the
   [configuration reference](../reference/configuration.md) for exact names,
   types, and precedence.
2. **A missing or malformed source tree.** A non-existent source root, or a
   decrypted JSON or YAML document that is not a single top-level object (a JSON
   object, or a single YAML document with a top-level mapping), contains
   duplicate keys, is not UTF-8, exceeds the canonical size limit, or fails
   SOPS MAC verification. Correct the committed files and push; the constraints
   are catalogued in the
   [reconciliation reference](../reference/reconciliation.md).
3. **An empty desired state with pending deletions and no authorization.** If a
   commit leaves zero source files and the plan would schedule deletions, the
   run is blocked with `status: invalid` and mutates nothing. This is the
   empty-scope safety gate, not a failure to repair by re-running. If the empty
   state is unintended, restore the missing files; if it is intentional, follow
   [how to decommission or empty a scope](decommission-a-scope.md).
4. **A report-file write failure.** If the tool cannot write `--report-file`,
   it forces exit 3 even when the underlying run would have finished
   differently — the write failure masks the true outcome. Ensure the report
   directory exists and is writable, then re-run to see the real result.

## Exit 4 — ownership or observed-state conflict

Action message: `sops-aws-sync found an ownership or observed-state conflict`.

A single conflicting secret blocks the entire plan before any mutation. First
identify *which* secret and *which* conflict class. Resource names are redacted
to `resource-NNNN` ordinals by default; to reveal the real name, re-run with
`--show-resource-names` (Action input `show-resource-names: true`) into a
trusted log sink only ([tradeoff](../explanation/security-and-trust.md)). The
conflict-class catalog and each triggering condition live in the
[reconciliation reference](../reference/reconciliation.md).

Remediate by class. The class token appears in `--show-resource-names` output
and is catalogued in the
[reconciliation reference](../reference/reconciliation.md); the eight classes
map to three remedies:

- **Ownership or source mismatch** (`ownership`, `source`) — a same-name secret
  the tool does not own, or an owned secret whose `source` tag points at a
  different desired document. The tool never adopts, overwrites, or retags a
  secret it does not own, and ownership tags are written once at create time —
  there is no retag path. Rename the source file, or choose a different
  `secret-prefix`, so the desired name no longer collides.
- **Service-owned, rotation, replication, or ambiguous staging**
  (`service-owned`, `rotation`, `replication`, `staging`) — an owned secret AWS
  will not let the tool touch safely. Resolve the condition in AWS first —
  migrate off the owning service, disable rotation, remove replicas, or clear
  the ambiguous staging label — then re-run.
- **Malformed source tag or payload** (`source-tag`, `payload`) — the secret's
  own metadata is corrupt: its `source` tag is present but is not a valid
  SHA-256 digest, or its current version is neither a clean string nor a clean
  binary value. The tool cannot repair either state safely; correct the
  secret's tag or version directly in AWS, then re-run.

A missing IAM permission does *not* reach exit 4: any access-denied fault
short-circuits during observation and surfaces as exit 5 with status
`observation-failed`, handled below.

## Exit 5 — apply or observation failed

Action message: `sops-aws-sync apply did not complete safely`.

A mutation or an observation call failed, or a delete/restore returned an
ambiguous result. Report `status` is `apply-failed` or `observation-failed`.

1. **Re-run `sync`.** No rollback occurred; the next run resumes from observed
   state. Create and update ambiguity self-resolves on re-observation because
   those operations carry an idempotency token. An ambiguous restore or
   scheduled deletion carries no token and fails safe, so it requires your
   deliberate re-run ([why](../explanation/consistency-and-recovery.md)).
2. **If it persists, open an AWS support case.** The JSON logs carry a safe AWS
   error code, fault class, and request ID (see the
   [results reference](../reference/results.md)) — use them without needing the
   raw error text.
3. **If the fault is access-denied**, add the missing IAM action per
   [how to grant AWS access](grant-aws-access.md).

## Exit 6 — verification did not converge

Action message: `sops-aws-sync verification did not converge`.

Sync applied changes but could not confirm convergence. The `verification`
value tells you which case you are in:

- **`inconclusive`** — the writes most likely landed, but AWS did not stabilize
  within the verification window. Raise `--verification-timeout` (Action input
  `verification-timeout`) and/or re-run. Do **not** re-apply from scratch.
- **`failed`** — a real mismatch, or the bounded self-healing rebuild budget was
  exhausted by sustained concurrent change. Confirm exactly one writer is active
  for this scope, then re-run
  ([why a single writer matters](../explanation/consistency-and-recovery.md)).

## Exit 130 — interrupted

Action message: `sops-aws-sync was interrupted`.

The process received SIGINT/SIGTERM or hit the run timeout. This maps the same
whether it happened during setup, SOPS decryption, or apply. No AWS mutation
begins until every document decrypts, so an interruption during setup left
nothing partially applied.

1. **Re-run.** The recovery path is identical to any other partial run.
2. **If it recurs at setup**, raise `--run-timeout` (Action input `run-timeout`)
   or check that SOPS key backends (KMS, age, and so on) are reachable from the
   runner.

## Action messages without an exit code

Some Action failures are raised by the Action wrapper itself, not by a CLI exit
code:

- **`CLI exit status and redacted report disagree`** — the numeric exit code
  and the report `status` did not match, or the CLI exited with a code outside
  the documented set. This signals version skew between the pinned Action ref
  and the `cli-version` it installed. Align the pinned release ref with
  `cli-version` so the two move in lockstep
  ([why they are locked together](../explanation/security-and-trust.md)).
- **`Exact CLI installation or verification failed`** — the Action could not
  install the pinned CLI binary or verify it *before* the CLI ever ran: a failed
  download, a checksum mismatch, missing or malformed release checksums, an
  incompatible bundled GitHub CLI, or unverifiable release/attestation metadata.
  Most such faults normalize to this catch-all message; a more specific variant
  (for example `CLI binary checksum verification failed`) may appear instead.
  Confirm the runner meets the platform and environment requirements in the
  [configuration reference](../reference/configuration.md), and review the trust
  model in [about security and trust](../explanation/security-and-trust.md).
- **`sops-aws-sync Action execution failed`** — a generic internal failure the
  Action did not anticipate (an unexpected bug, not a normalized install or CLI
  condition). Re-run; if it persists, review the trust model in
  [about security and trust](../explanation/security-and-trust.md).
- **`sops-aws-sync plan detected drift`** (exit 2) — this is not a failure to
  recover from. It is the intended drift gate: a `plan` run with drift gating
  enabled found executable changes and failed the check on purpose. See the
  [results reference](../reference/results.md) for the exact conditions that
  produce it.

## Confirm recovery

After any fix, run `plan` and check the result:

    sops-aws-sync plan --repository-id … --secret-prefix … --aws-region …

A `converged` status with zero create, update, restore, and scheduled-delete
counts confirms the scope is healthy. Because a converged scope is a pure no-op,
this check is safe to repeat as often as you like.

## Related

- [Results and exit codes reference](../reference/results.md) — what each exit
  code, status, and verification value means.
- [Reconciliation reference](../reference/reconciliation.md) — the conflict-class
  catalog behind exit 4.
- [About consistency, verification, and recovery](../explanation/consistency-and-recovery.md)
  — why ambiguous writes and inconclusive verification happen.
- [About the security and trust model](../explanation/security-and-trust.md) —
  why error detail is withheld and versions are pinned.
- [How to decommission or empty a scope safely](decommission-a-scope.md) —
  intentional teardown, which is not a failure.

# How to decommission or empty a scope safely

Retire individual secrets or an entire scope through the tool's soft-delete
path without triggering accidental mass deletion, and recover within the
recovery window if a deletion was unintended.

## Prerequisites

- Write access to commit and push to the source repository.
- A working `sync` path already configured against the target scope — the
  same `repository-id`, `source-root`, and `secret-prefix` you normally
  reconcile with, run either from the CLI or through the GitHub Action.

Retirement happens only through `sync`. Deleting a file changes desired
state; the deletion is executed the next time `sync` runs against the commit
that removed it. The examples below show the CLI; the Action performs the
same operations with the equivalent inputs (see
[Configuration reference](../reference/configuration.md)).

## Retire a single secret

Removing one committed `.sops.json` while others remain schedules that one
secret for deletion.

### 1. Remove the source file and commit

    git rm secrets/payments/legacy.sops.json
    git commit -m "Retire legacy payments secret"
    git push

### 2. Reconcile the commit

    sops-aws-sync sync \
      --revision HEAD \
      --repository-id you/app \
      --source-root secrets \
      --secret-prefix /acme/payments \
      --aws-region us-east-1

The tool reads the committed blobs at `--revision`, sees the secret is owned
but no longer desired, and schedules it for deletion. Scheduled deletions run
**last** — after every restore, create, and update in the same run — so a
retirement never removes access before other changes land.

Because at least one desired file remains, this is a deletion from a
non-empty desired snapshot, which is **not** gated. No extra authorization is
needed.

### 3. Verify

Confirm a scheduled-deletion count of `1` and `status: converged` — the field
is `counts.schedule_delete` in the report and `scheduled-delete-count` in the
Action outputs. For what each status and exit code means, see
[Results and exit codes reference](../reference/results.md).

## Empty an entire scope

Retiring *every* secret in a scope is deliberately guarded, because a commit
that accidentally leaves zero desired files would otherwise schedule the whole
scope for deletion.

### 1. Keep the source root present

Git cannot track an empty directory, so hold the source root open with a
tracked placeholder such as `secrets/.gitkeep`. If you instead `git rm` every
tracked file under `source-root`, Git removes the directory itself from the
committed tree, and the run is rejected before the empty-state authorization is
ever evaluated — a missing source root and a deliberately empty one are
distinct states (see
[Reconciliation reference](../reference/reconciliation.md#empty-desired-state-gate)
for why a missing source root is rejected first).

Commit a placeholder that is not itself a source document alongside the
removals:

    git rm secrets/payments/*.sops.json
    touch secrets/.gitkeep
    git add secrets/.gitkeep
    git commit -m "Empty payments scope"
    git push

The placeholder keeps the directory in the committed tree. It does not end in
`.sops.json`, so it is ignored as a desired document and the desired snapshot
is genuinely empty.

### 2. Authorize the empty transition

A commit that leaves zero desired files and whose plan would schedule
deletions is blocked with `status: invalid` and zero deletions unless you
explicitly authorize it. Pass `--allow-empty` on the CLI (or `allow-empty:
'true'` on the Action); for the full flag definition, environment binding, and
precedence, see [Configuration reference](../reference/configuration.md):

    sops-aws-sync sync \
      --allow-empty \
      --revision HEAD \
      --repository-id you/app \
      --source-root secrets \
      --secret-prefix /acme/payments \
      --aws-region us-east-1

For why the empty state is treated as dangerous, see
[About the reconciliation model](../explanation/reconciliation-model.md).

!!! note
    The authorization is one-shot for the transition. Once the secrets are
    already scheduled for deletion, later runs converge without
    `--allow-empty`: an owned secret that is already scheduled counts as
    unchanged, so subsequent plans contain no new deletions to gate.

### 3. Verify

Confirm that the scheduled-deletion count (`counts.schedule_delete` in the
report, `scheduled-delete-count` in the Action outputs) equals the number of
retired secrets and that `status: converged`.

## Choose the recovery window

Deletion is always an AWS scheduled deletion inside a recovery window; there
is no permanent or force delete. Set the window with `--recovery-window-days`
(Action input `recovery-window-days`), an integer from `7` to `30` that
defaults to `30` and is passed verbatim to AWS:

    sops-aws-sync sync \
      --recovery-window-days 7 \
      --revision HEAD \
      --repository-id you/app \
      --source-root secrets \
      --secret-prefix /acme/payments \
      --aws-region us-east-1

The window applies to every deletion scheduled in that run. For the full flag
definition, environment binding, and precedence, see
[Configuration reference](../reference/configuration.md).

## Recover a secret within the recovery window

While a secret is still within its recovery window, restore it by putting the
source file back and reconciling.

### 1. Re-add the file at the same path

Restore the file at the exact path it had before, so it derives the same AWS
secret name:

    git checkout HEAD~1 -- secrets/payments/legacy.sops.json
    git commit -m "Restore legacy payments secret"
    git push

### 2. Reconcile

    sops-aws-sync sync \
      --revision HEAD \
      --repository-id you/app \
      --source-root secrets \
      --secret-prefix /acme/payments \
      --aws-region us-east-1

The tool cancels the scheduled deletion, restoring the secret rather than
recreating it, and reconciles it to its committed value within the same run.
Expect a `restore` count of `1`.

If the recovery window has already elapsed, AWS has deleted the secret and
there is nothing to restore: re-adding the file produces a fresh `create`
instead. For why restore is a separate cycle from the value update, see
[About the reconciliation model](../explanation/reconciliation-model.md).

## When a scheduled deletion is ambiguous

If a `sync` ends with `status: apply-failed` (exit `5`) because a scheduled
deletion returned an ambiguous result, the run stops with no same-run retry.
Re-run `sync` — no rollback occurred and the next run resumes from the
observed state. For why delete and restore fail safe rather than retrying
automatically, see
[About consistency, verification, and recovery](../explanation/consistency-and-recovery.md).

## Cautions

- **Renaming is not renaming.** Moving or renaming a source file changes both
  the derived secret name and its source identity, so the tool sees
  delete-old plus create-new — a `schedule-delete` and a `create`, never a
  rename. See
  [About the reconciliation model](../explanation/reconciliation-model.md).

- **Changing scope inputs abandons, it does not delete.** Changing
  `repository-id`, `source-root`, or `secret-prefix` mints a new scope that
  owns nothing prior. The old secrets are left in AWS, unmanaged — they are
  *abandoned*, not scheduled for deletion. This is not a way to decommission a
  scope. See
  [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).

## If a deletion was unintended

Act before the recovery window elapses. Follow
[Recover a secret within the recovery window](#recover-a-secret-within-the-recovery-window)
to re-add the file and restore. Verify in the AWS console that the secret
still shows *scheduled for deletion* rather than already gone; once the window
passes, only a fresh create is possible.

For a symptom-indexed guide to any failed or non-converged run, see
[How to diagnose and recover from a failed run](diagnose-and-recover.md).

## Related

- [Reconciliation reference](../reference/reconciliation.md) — the decision
  matrix, phase order, and the empty-state gate as rules.
- [Results and exit codes reference](../reference/results.md) — what
  `invalid`, `apply-failed`, and each exit code mean.
- [Configuration reference](../reference/configuration.md) —
  `--allow-empty`, `recovery-window-days`, and every other option.
- [About the reconciliation model](../explanation/reconciliation-model.md) —
  why deletion is soft and restore is a two-cycle operation.
- [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md)
  — why changing scope inputs abandons secrets.

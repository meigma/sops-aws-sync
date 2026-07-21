---
title: "Tutorial: Your first reconciliation"
---

# Tutorial: Your first reconciliation

In this tutorial we will run the whole reconciliation loop once, by hand,
against a disposable scope. Starting from two committed `*.sops.json`
documents, we will preview a plan, apply it, watch a re-run do nothing, repair
a secret someone changed behind our back, then soft-delete one secret and bring
it back. By the end you will have watched `plan` and `sync` behave exactly as
the model promises, and you will trust what a converged run means.

Everything happens under a throwaway prefix like `/scratch/demo`, so nothing
here can touch real secrets. We keep two secrets committed the whole way
through, so the scope is never empty and we never reach the mass-deletion
guard.

## What we will build

We will drive a single AWS Secrets Manager scope through its full lifecycle:

1. Two secrets created from committed files.
2. A re-run that changes nothing.
3. One secret repaired after an out-of-band edit.
4. One secret scheduled for deletion, then restored.

Each step ends by reading a machine report, so you always see the outcome as
plain counts and a status — never guesswork.

## Prerequisites

We assume competence with Git, SOPS, and AWS, and we do not teach them here.
You need:

- A local Git checkout you can commit to (any repository will do).
- A working SOPS key and the ability to produce encrypted `*.sops.json`
  documents. If you need it, see the
  [SOPS project](https://github.com/getsops/sops).
- Throwaway AWS credentials for one region, resolvable through the standard AWS
  SDK chain. The tool reads no credential inputs of its own. For the real
  least-privilege policy, see
  [how to grant AWS access](../how-to/grant-aws-access.md).
- The verified `sops-aws-sync` CLI on your `PATH`. If you downloaded a binary
  directly, first
  [verify the release binary](../how-to/verify-a-release-binary.md).

Confirm the CLI is installed:

```console
$ sops-aws-sync version
sops-aws-sync 1.2.3 (a1b2c3d) built 2026-07-20T00:00:00Z
```

The exact version, commit, and date depend on the release you installed; any
line in this shape confirms a working binary.

## Step 1: Commit two secrets

Create two SOPS documents, each a single top-level JSON object. Use this
plaintext:

`alpha` (decrypted):

```json
{ "demo": "alpha-initial" }
```

`beta` (decrypted):

```json
{ "demo": "beta-initial" }
```

Encrypt each with your SOPS key so the committed files are the encrypted
documents, laid out under a `secrets/` directory:

```
secrets/
├── alpha.sops.json
└── beta.sops.json
```

Then commit them:

```console
$ git add secrets/alpha.sops.json secrets/beta.sops.json
$ git commit -m "Add alpha and beta secrets"
```

!!! note "Commit is the source of truth"

    The tool reads the encrypted blobs committed at the revision you pass —
    never your working tree, index, or unpushed changes. If you skip the
    commit, the next command sees nothing. This is the one habit the whole
    model rests on; the
    [reconciliation model](../explanation/reconciliation-model.md) explains
    why.

We committed two secrets, not one, on purpose. Later we delete `alpha` while
`beta` stays committed, so the scope never becomes empty and the path stays
the same from start to finish.

## Step 2: Preview the plan

`plan` is read-only. It reports what a `sync` would do without touching AWS.
Run it, writing the machine report to `plan.json`:

```console
$ sops-aws-sync plan \
    --repository . \
    --revision HEAD \
    --repository-id you/demo \
    --source-root secrets \
    --secret-prefix /scratch/demo \
    --aws-region us-east-1 \
    --report-file plan.json
{"time":"2026-07-20T00:00:00Z","level":"INFO","msg":"plan complete","phase":"plan","status":"drift","count":2,"duration_ms":42}
$ echo $?
0
```

`plan` streams structured JSON logs to stdout. The single `plan complete` line
reports `count: 2` — two planned operations, one for each committed file — and a
`drift` status because AWS does not yet match Git. The command exits `0` and
creates nothing in AWS. Use your own throwaway region in place of `us-east-1`;
every later command reuses this same set of scope flags. (The `time` and
`duration_ms` values vary by run.)

## Step 3: Read the redacted report

Open `plan.json`:

```json
{
  "schema_version": "sops-aws-sync/report/v1",
  "tool_version": "1.2.3",
  "git_revision": "6f1e2d3c4b5a69788796a5b4c3d2e1f0a9b8c7d6",
  "status": "drift",
  "counts": {
    "create": 2,
    "update": 0,
    "restore": 0,
    "schedule_delete": 0,
    "unchanged": 0,
    "conflicts": 0
  },
  "verification": "not-run",
  "duration_ms": 42
}
```

Notice what is here: `status` is `drift` because AWS does not yet match Git,
and `counts.create` is `2` — one secret for each committed file. `verification`
is `not-run` because `plan` never mutates, so there is nothing to verify.

Now notice what is *not* here. The report carries counts, a status, the
resolved commit SHA, and timing — and no secret values, no secret names, and no
source paths. That is why this file is safe to attach to a ticket or paste into
a chat. (`tool_version`, `git_revision`, and `duration_ms` vary by run; the rest
will match.)

## Step 4: Apply with sync

Now make AWS match Git. The command is the same, with `sync` instead of `plan`
and a fresh report file:

```console
$ sops-aws-sync sync \
    --repository . \
    --revision HEAD \
    --repository-id you/demo \
    --source-root secrets \
    --secret-prefix /scratch/demo \
    --aws-region us-east-1 \
    --report-file report.json
{"time":"2026-07-20T00:00:00Z","level":"INFO","msg":"reconciliation operation","phase":"apply","operation":"create","status":"applying","resource":"resource-0001"}
{"time":"2026-07-20T00:00:00Z","level":"INFO","msg":"reconciliation operation","phase":"apply","operation":"create","status":"applying","resource":"resource-0002"}
$ echo $?
0
```

Look at the two `reconciliation operation` lines on stdout: each names its
secret by an ordinal — `resource-0001`, `resource-0002` — never by its real
name, the same redaction you saw in the report.

Open `report.json`:

```json
{
  "status": "converged",
  "counts": { "create": 2, "update": 0, "restore": 0,
              "schedule_delete": 0, "unchanged": 0, "conflicts": 0 },
  "verification": "converged"
}
```

Two secrets were created, `status` is `converged`, and — this is the part to
watch — `verification` is `converged` too. `sync` always re-observes the scope
after applying and confirms it converged before reporting success; there is no
apply-without-verify mode. The exit code is `0`.

## Step 5: Re-run sync and change nothing

Run the exact same `sync` again. We committed nothing new, so AWS already
matches Git:

```console
$ sops-aws-sync sync \
    --repository . --revision HEAD --repository-id you/demo \
    --source-root secrets --secret-prefix /scratch/demo \
    --aws-region us-east-1 --report-file report.json
$ echo $?
0
```

`report.json`:

```json
{
  "status": "converged",
  "counts": { "create": 0, "update": 0, "restore": 0,
              "schedule_delete": 0, "unchanged": 2, "conflicts": 0 },
  "verification": "converged"
}
```

Every executable count is `0`; both secrets are `unchanged`. A converged scope
is a pure no-op. This is exactly why `plan` is safe to run on every pull
request as a drift check.

## Step 6: Repair external drift

Now let us change a secret behind the tool's back. In the AWS Secrets Manager
console, open the secret named `/scratch/demo/alpha`, retrieve its value, and
edit it to something different, for example:

```json
{ "demo": "changed-in-the-console" }
```

Save it. We have *not* committed anything — Git still says
`{ "demo": "alpha-initial" }`. Run `sync` again:

```console
$ sops-aws-sync sync \
    --repository . --revision HEAD --repository-id you/demo \
    --source-root secrets --secret-prefix /scratch/demo \
    --aws-region us-east-1 --report-file report.json
$ echo $?
0
```

`report.json`:

```json
{
  "status": "converged",
  "counts": { "create": 0, "update": 1, "restore": 0,
              "schedule_delete": 0, "unchanged": 1, "conflicts": 0 },
  "verification": "converged"
}
```

`counts.update` is `1`: `sync` noticed that `alpha` no longer matched Git and
rewrote it to the committed value, while `beta` stayed `unchanged`. The commit
never changed — the drift was repaired because this secret carries the tool's
ownership tags, marking it as ours to manage. The
[ownership and scope](../explanation/ownership-and-scope.md) model explains what
makes a secret ours.

## Step 7: Remove one secret

Delete `alpha` from Git and commit, leaving `beta` in place:

```console
$ git rm secrets/alpha.sops.json
$ git commit -m "Remove alpha secret"
$ sops-aws-sync sync \
    --repository . --revision HEAD --repository-id you/demo \
    --source-root secrets --secret-prefix /scratch/demo \
    --aws-region us-east-1 --report-file report.json
$ echo $?
0
```

`report.json`:

```json
{
  "status": "converged",
  "counts": { "create": 0, "update": 0, "restore": 0,
              "schedule_delete": 1, "unchanged": 1, "conflicts": 0 },
  "verification": "converged"
}
```

`counts.schedule_delete` is `1`: `alpha` is no longer in Git, so the tool
scheduled it for deletion, while `beta` remained `unchanged`. Because `beta` is
still committed, the desired state is not empty, so the deletion proceeds
normally. The deletion is soft — AWS holds the secret inside a recovery window,
so it is still restorable, which we prove next.

## Step 8: Restore the secret

Bring `alpha` back at the same path, this time with a new value. Use this
plaintext:

```json
{ "demo": "alpha-restored" }
```

Encrypt it with your SOPS key as before, then commit and reconcile:

```console
$ git add secrets/alpha.sops.json
$ git commit -m "Restore alpha secret"
$ sops-aws-sync sync \
    --repository . --revision HEAD --repository-id you/demo \
    --source-root secrets --secret-prefix /scratch/demo \
    --aws-region us-east-1 --report-file report.json
$ echo $?
0
```

`report.json`:

```json
{
  "status": "converged",
  "counts": { "create": 0, "update": 1, "restore": 1,
              "schedule_delete": 0, "unchanged": 1, "conflicts": 0 },
  "verification": "converged"
}
```

Because the deletion was still inside its recovery window, `alpha` was
*restored* rather than created fresh — `counts.restore` is `1`. Restore is a
two-step cycle: the tool first cancels the scheduled deletion, then, because the
committed value changed while `alpha` was gone, the same run writes the new
value into the restored secret, so `counts.update` is `1` too. (Had you brought
`alpha` back with the exact value it held before deletion, the restore alone
would have converged it and no update would appear.) `beta` stayed `unchanged`,
and the scope converged again.

## What we learned

You drove one scope through its entire lifecycle and confirmed each behavior by
reading a report:

- **Git is the desired state.** Nothing happens until you commit; `sync` reads
  the committed revision.
- **`plan` is always safe.** It is read-only and mutates nothing.
- **`sync` mutates, then verifies.** Success means the scope was independently
  confirmed to have converged.
- **Re-runs are no-ops.** A converged scope produces zero operations.
- **Drift is repaired.** An owned secret changed out of band is rewritten to
  the committed value.
- **Deletion is soft and reversible.** Removing a file schedules deletion inside
  a recovery window; re-adding it within the window restores it.

## Next steps

- **Why it behaves this way** —
  [About the reconciliation model](../explanation/reconciliation-model.md)
  explains desired-vs-observed state, convergence, and the soft-delete
  lifecycle you just watched.
- **Run it for real** —
  [How to deploy sops-aws-sync with GitHub Actions](../how-to/deploy-with-github-actions.md)
  wires this loop into CI with plan-on-PR and sync-on-merge.
- **Retire secrets on purpose** —
  [How to decommission or empty a scope safely](../how-to/decommission-a-scope.md)
  covers intentional teardown, including how to safely empty an entire scope,
  which this tutorial deliberately never did.
- **Look up a flag** — the
  [configuration reference](../reference/configuration.md) catalogs every flag,
  default, and constraint.

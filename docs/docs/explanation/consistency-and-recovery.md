# About consistency, verification, and recovery

A `sync` does not finish when the tool has issued its AWS writes. It finishes
when the tool has independently re-observed the scope and confirmed that AWS now
matches the committed desired state. That extra step exists because the ground
the tool stands on is not solid: AWS Secrets Manager is eventually consistent
and offers no transaction across secrets. A write can succeed while the rest of
the API still reports the old world, and a network hiccup can leave the tool
genuinely unsure whether a write landed at all.

This document explains the consequences of operating on that ground: why a sync
can end as *inconclusive* rather than succeed or fail cleanly, why re-running is
almost always the right recovery, why correctness depends on there being exactly
one writer per scope, and why an interrupted run is safe to simply run again.
If you have hit a non-converged result and want the specific remedy, go to
[how to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md);
this page is the reasoning behind those remedies. For the happy-path decision
logic — how desired and observed state become a plan — see
[about the reconciliation model](reconciliation-model.md).

## Eventual consistency is the root cause

Everything else here follows from one property of the platform. When the tool
lists secrets, that list is eventually consistent: a secret you just created,
updated, or deleted may not yet appear — or may still appear with stale
metadata — for a short window after the write returns success. Amazon documents
this lag, and in practice it can last from a fraction of a second to a few
minutes.

The tool cannot trust the list as a snapshot of truth, so it never does. Scope
discovery uses the list only to *find candidate names* carrying the tool's
ownership tag. Every name that matters — every committed desired name, and every
candidate the list turned up — is then observed directly, one secret at a time,
with a describe call and, where a value comparison is needed, a fetch of the
current version. The tool also cross-checks staging: if a secret does not expose
exactly one current version, or the directly fetched version disagrees with what
the describe reported, that inconsistency is treated as a reason to stop rather
than a detail to paper over.

The practical meaning is worth stating plainly: a successful sync is not a
claim that AWS is in a globally strongly-consistent state at that instant. It is
a claim that, on direct and repeated observation, every secret the tool is
responsible for converged to its committed value. That is a weaker and more
honest guarantee, and it is the strongest one the platform actually supports.

## Why sync verifies its own work

Because the list can lag and a write's success does not immediately propagate,
the tool refuses to equate "I issued the operations" with "the state is
correct." After applying a plan, `sync` re-observes the entire scope and rebuilds
the plan from scratch. If the rebuilt plan is empty, the scope has converged and
the run succeeds. The tool repeats this re-observation on a fixed sub-second
poll until convergence, bounded by a configurable verification timeout, and it
explicitly re-observes every name it just wrote even when eventually-consistent
discovery still omits that name. There is no apply-without-verify mode; the
verification is part of what `sync` *is*.

That verification produces one of four outcomes, and understanding them
conceptually is what lets you react correctly. (The exact status strings live in
the [results reference](../reference/results.md); the point here is what each
one *means*.)

- **Converged** — re-observation confirmed the committed state. This is success.
- **Inconclusive** — the writes very likely landed, but AWS did not stabilize
  into an observably converged state before the verification timeout expired.
  This is a statement about propagation delay, not about correctness. The remedy
  is a longer timeout or a plain re-run, never a re-apply from scratch.
- **Failed** — re-observation still shows a real mismatch, or a fresh conflict
  appeared, or the tool's own self-healing budget (below) was exhausted. This
  means the desired state was genuinely not reached and something needs
  attention — most often a second writer.
- **Not run** — the run ended before verification could begin (a conflict, an
  apply failure, or an interruption stopped it earlier).

The distinction between *inconclusive* and *failed* is the most common source of
operator over-reaction. Inconclusive is not failure. Tearing down and rebuilding
a scope in response to an inconclusive result is exactly the wrong move: the
writes probably succeeded, and re-observing them a moment later will usually show
convergence.

## Ambiguous writes, and why the tool treats operations asymmetrically

Verification handles the case where writes succeeded but propagation lagged. A
different problem is a write whose *outcome itself* is unknown — the request
timed out, the connection dropped, or the context was cancelled mid-call. The
tool may have changed AWS, or may not have. Re-issuing blindly risks a double
action; assuming failure risks reporting a false error. Neither is acceptable, so
the tool re-observes reality before deciding.

How it decides depends on the operation, and the asymmetry is deliberate:

- **Create and update** each carry a fresh, single-use idempotency token. On an
  ambiguous result the tool re-observes the secret and then either accepts the
  state as already correct, retries exactly once with the *same* token — so a
  duplicate that AWS already applied is de-duplicated rather than doubled — or,
  if it still cannot prove safety, fails closed. Because these writes are
  naturally idempotent under a client token, an ambiguous create or update can
  usually resolve itself within the same run.
- **Restore and schedule-deletion** carry no token. There is no safe way to make
  "cancel a scheduled deletion" or "schedule a deletion" idempotent against an
  uncertain first attempt, so the tool never re-issues one. It still re-observes
  the secret first: if that observation proves the restore or deletion actually
  took effect, the tool accepts the state and the run continues. Only when it
  cannot prove the outcome does an ambiguous restore or deletion fail closed as
  an apply failure — with no same-run retry — leaving you to re-run deliberately
  once you can see the secret's real state.

This is a considered trade-off, not an inconsistency. The tool accepts any write
it can observe has already landed, additionally repeats only the operations it
can prove are safe to repeat, and refuses to gamble on the rest. When a delete or
restore comes back ambiguous and cannot be observed as done, the safe recovery is
to look at the secret in AWS and run `sync` again; the second run observes the
settled state and does the right thing.

## Why external drift is repaired even when nothing was committed

A subtle consequence of per-run tokens is that the tool repairs drift it did not
cause. Suppose someone edits a managed secret's value directly in the AWS
console. The committed Git state has not changed, so you might expect the tool to
do nothing on the next run. Instead it sees that the live value no longer matches
the committed canonical value and writes the committed value back.

The reason this works cleanly is that each run mints its own idempotency tokens.
A token is never reused across runs, so a stale token from an earlier apply can
never cause AWS to silently swallow a later, legitimate rewrite. Every run is
free to re-assert the committed state. Combined with the byte-level value
comparison described in
[about the reconciliation model](reconciliation-model.md), this is why the tool
is a genuine reconciler: Git is the desired state, and each run drives AWS back
toward it regardless of what happened in between.

## Bounded self-healing, and the single-writer assumption

Between building a plan and applying each operation, the tool re-checks a
secret's state against a fingerprint of what it expected. Harmless drift — a
secret that changed in a benign way since planning — triggers a single, full
rebuild of the plan so the tool acts on current reality rather than a stale
picture. But it allows *at most one* such rebuild. If reality keeps shifting
underneath it, the tool does not loop forever chasing a moving target; it stops
and reports verification as failed.

That budget of one is where a hidden assumption becomes visible: the tool
assumes it is the only active writer for its scope. AWS Secrets Manager provides
no cross-secret transaction and no compare-and-swap that the tool could use to
serialize against a competitor. The fingerprint precondition protects a single
secret against a single benign change, but nothing at the platform level can make
two concurrent reconcilers of the same scope safe. Two writers racing will
produce churn that exhausts the rebuild budget and surfaces as a verification
failure — which is the tool telling you, as loudly as it safely can, that its
core assumption was violated.

!!! note "Enforce one writer per scope"
    Correctness depends on exactly one active `sync` per ownership scope at a
    time, run only from trusted, serialized contexts. In CI this is a
    concurrency group keyed to the scope; see
    [how to deploy with GitHub Actions](../how-to/deploy-with-github-actions.md)
    for the mechanism. A `verification-failed` result that recurs is a strong
    signal that a second writer exists.

## No rollback: sync is resumable-forward, not transactional

The tool never undoes a completed AWS call. There is no compensating-transaction
machinery, and there could not be a reliable one, because a partially-applied
change to eventually-consistent, individually-mutated secrets cannot be atomically
reversed. Instead the design leans into moving forward safely.

Two properties make forward-only recovery trustworthy. First, operations run in a
fixed, safety-oriented order — restores and creates and updates all precede any
scheduled deletion — so a failure partway through a run stops the tool *before*
it removes access to anything. Second, a later run does not resume from wherever
the previous run happened to stop; it re-observes actual state and plans afresh.
Completed writes are seen as done, incomplete ones are re-planned, and the scope
is driven the rest of the way to convergence.

This is why "re-run it" is the honest general answer to almost every failed
`sync`. The tool did not leave AWS in a half-rolled-back limbo; it left it in a
real, observable state that the next run can reason about and finish. Idempotent
creates and updates and a converged scope being a pure no-op mean re-running is
cheap and safe rather than risky.

## Interruption is safe by construction

Two things map to the same *interrupted* result: a signal (SIGINT or SIGTERM,
for example a cancelled CI job or an operator pressing Ctrl-C) and the run
exceeding its overall run timeout. They share one outcome because, from a
recovery standpoint, they are the same situation — the run stopped early through
no fault of the desired state — and both are safely recovered by running again.
That is also why an interruption during setup and a run-timeout during setup are
indistinguishable by exit code: the correct response to both is identical.

Interruption is safe largely because of *when* the tool does its dangerous work.
The tool loads and decrypts every committed document up front, before it makes a
single mutating AWS call. If a decryption fails, or the run is cancelled during
decryption, the run ends before any secret has been touched. There is one sharp
edge worth knowing: decryption delegates to SOPS, whose call into your key
provider cannot be cooperatively cancelled. If you interrupt the process mid-
decrypt, the tool stops waiting and exits, but the in-flight provider call (a KMS
decrypt, say) is abandoned rather than gracefully aborted. No AWS *secret*
mutation has begun at that point, so the interruption is still clean from the
scope's perspective; the consequence is only that the process may exit a moment
after you asked it to.

Because no mutation starts until every decryption has succeeded, an interrupted
run can never have applied half of a plan built from documents that partly
failed to decrypt. Either the whole desired state was readable and the tool
proceeded, or it stopped before touching AWS.

## What a converged sync does and does not promise

A converged sync tells you that, at the moment of verification, every secret in
the scope was directly observed to hold its committed value, that no conflict
blocked the plan, and that any ambiguity resolved in favor of a proven-correct
state. It does not promise that AWS will forever agree instantaneously with every
API — eventual consistency is a property of the platform, not a bug the tool can
fix — and it does not promise anything about a scope that a second writer is
concurrently mutating.

Operate accordingly: run one writer per scope, treat *inconclusive* as "wait and
re-observe" rather than "start over," treat *interrupted* as "run it again," and
reach for a teardown only when *failed* persists after you have confirmed you are
the only writer. The tool is built so that the safe action is almost always the
simple one.

## Related reading

- [How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md)
  — the symptom-to-remedy guide these concepts justify.
- [Results and exit codes reference](../reference/results.md) — the exact exit
  codes, statuses, and verification values named conceptually here.
- [About the reconciliation model](reconciliation-model.md) — the desired-vs-
  observed decision logic, value comparison, and operation ordering.
- [About the security and trust model](security-and-trust.md) — why errors are
  opaque and why secrets never reach logs or reports.
- [How to deploy with GitHub Actions](../how-to/deploy-with-github-actions.md) —
  configuring single-writer concurrency in a real workflow.
- [Configuration reference](../reference/configuration.md) — the run-timeout and
  verification-timeout names, defaults, and syntax.

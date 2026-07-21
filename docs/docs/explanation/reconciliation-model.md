# About the reconciliation model

Most tools that move secrets into AWS think of themselves as a pump: you
hand them a value, they push it to Secrets Manager. `sops-aws-sync` is not
a pump. It is a reconciler. It never asks "what value did you give me?"
and instead asks "what does the committed source say the world should
look like, what does AWS actually look like right now, and what is the
smallest, safest set of changes that closes the gap?"

That distinction is the whole model, and it is worth internalising before
you point the tool at production secrets. A pump is only as trustworthy as
the last command someone typed. A reconciler is trustworthy because its
behaviour is a function of state you can read, review, and reproduce. When
a plan surprises you, the model is what lets you work out why — and it is
almost always right, in a way that a one-off imperative command never can
be. This page explains how the tool decides what to change and what it
treats as already done. The exact rules that make those decisions
predictable — the full decision matrix, the naming grammar, the size and
character limits — live in the [reconciliation
reference](../reference/reconciliation.md); here we explain the reasoning
those rules encode.

## Git is the desired state, and only Git

The desired state is the set of committed `*.sops.json` documents at one
exact commit. Nothing else. The working tree is never read; the staging
index is never read; a file you have edited but not committed does not
exist as far as the tool is concerned. This is a deliberate narrowing, and
it is the single most important thing to hold in your head: you change AWS
by changing Git, and only Git.

The practical consequence is that a reconciliation only ever acts on
something that has been committed. In the CI workflow the tool is built
for, that "something" is the commit the pipeline checked out and that a
merge pushed to the protected branch — so in practice the loop is commit,
push, reconcile. A local run reads your local repository's committed
objects at whatever revision you name, but it still ignores your uncommitted
edits entirely. If a `sync` did nothing you expected, the first question is
never "did the tool see my change?" but "is my change actually committed at
the revision I reconciled?"

AWS sits on the other side of this relationship as the *observed* state. It
is authoritative about nothing. The tool reads it to find out what already
exists, but it never treats a value living in Secrets Manager as a source
of truth to be preserved for its own sake. If the committed document and
the live secret disagree, Git wins, every time — that is what "reconcile
toward Git" means.

Because "the revision" has to be exact for any of this to be reproducible,
whatever you pass — a branch, a tag, `HEAD`, a short SHA — is resolved once
to a full 40-character commit identifier before reconciliation begins, and
that resolved commit is recorded with the run. A report therefore always
names precisely which revision it reconciled, which is what makes a past
run auditable rather than merely "the state of `main` at some point." The
trust properties of that provenance are the subject of [the security and
trust model](security-and-trust.md); here it is enough to know the commit
is pinned and never floats mid-run.

## A plan is a pure function of two inputs

Given the desired documents and the observed AWS state, producing a plan is
pure computation. The component that performs it is deliberately isolated:
it knows nothing about AWS, Git, or SOPS, and reaches for none of their
APIs. It takes two in-memory descriptions of state and returns a sorted list
of operations. That isolation is not an implementation detail you can safely
ignore — it is the reason a plan is trustworthy. Identical desired and
observed inputs always yield an identical plan, in identical order, with no
hidden dependence on network timing, wall-clock, or the machine it runs on.

Determinism is exactly what lets `plan` serve as a read-only preview and a
CI drift gate. A `plan` that reports no work to do is a genuine promise
about what a `sync` would do, not a hopeful guess, because the same pure
function drives both. Nothing about `plan` reaches out and changes AWS;
`sync` simply takes the same decisions and executes them, then verifies.

One subtlety follows from framing the plan as a function of two known sets.
The tool reasons only about names it can name: the union of the documents
Git wants and the secrets it already owns, discovered by their ownership
tags. It never enumerates the unbounded space of secrets that are neither
desired nor owned — it cannot, and it does not try. A name that is absent
from Git and not part of the tool's own scope is simply outside the
conversation. This is why the tool can never "accidentally" touch a secret
it was never managing: such a secret is never even a candidate.

## What the tool decides, described as decisions

For every name in that union, the plan reaches one decision. Rather than
reproduce the decision table — that belongs in the
[reference](../reference/reconciliation.md) — it is more useful to
understand the shape of the reasoning.

For a name that appears in Git, the question is what already exists at that
name. If nothing exists, the tool creates it. If an owned secret already
holds exactly the committed value, there is nothing to do. If an owned
secret exists but its value has drifted, or it holds a form the tool cannot
treat as a match, it is updated toward the commit. If an owned secret at
that name is currently scheduled for deletion, the tool restores it rather
than starting over. Every one of those branches presumes ownership; a
same-name secret the tool does not own is a different story entirely.

For a name the tool *owns* but that no longer appears in Git, the desired
state is saying "this should no longer exist," and the tool schedules it for
deletion. This is how removing a committed file removes a secret — not by a
separate delete command, but as the natural consequence of the file no
longer being part of desired state.

And for anything the tool encounters that it cannot safely act on — a
same-name secret it does not own, one whose ownership tags are wrong or
missing, or one in an AWS state the tool refuses to touch — the plan records
a conflict. A conflict is not a warning that gets stepped over; a single
conflict blocks the entire plan before any change is made. The reasoning
behind that fail-closed stance, and the precise boundary of what counts as
"owned," are the subject of [ownership, scope, and fail-closed
safety](ownership-and-scope.md), because they are a coherent topic in their
own right and the trap operators most often fall into.

## Names are derived, never stored

A secret's name is not something you configure per document and it is not
stored anywhere the tool later looks up. It is *derived*, deterministically,
from where the file sits in the repository: the configured prefix joined to
the document's path with the source root and the `.sops.json` suffix
removed, with the case preserved and no characters silently rewritten. The
file's location *is* its identity.

This has a consequence that surprises people, and it is worth stating
plainly because it is the classic "why did my plan do that?" moment.

!!! note "A rename is a delete plus a create"

    Because both the secret's name and its ownership identity are derived
    from the source path, moving or renaming a `.sops.json` file does not
    move or rename the secret. The tool sees the old path vanish from Git
    and a new path appear, and reconciles accordingly: the old secret is
    scheduled for deletion and a new one is created. There is no rename
    operation, and there cannot be, because nothing connects the two paths.

Understanding names as derived is also what makes it safe for the tool to
have no database of its own. It does not need to remember which file became
which secret; it can recompute that mapping from the source tree every time.
The exact path-to-name grammar, including the character set and length
limits a name must satisfy, is catalogued in the
[reference](../reference/reconciliation.md).

## Values are compared as canonical bytes

When an owned secret already exists, the tool has to decide whether its
stored value matches the commit. It does this the strictest way possible: it
compares the desired canonical JSON against the live value byte for byte.

This is a stronger test than "are these two documents semantically equal,"
and choosing it was deliberate. Every committed document is reduced to a
single canonical form before comparison, so two documents that differ only
in whitespace, key order, or number formatting collapse to the same bytes
and converge exactly once — after which they compare equal forever and never
churn. The flip side is the honest one: a value already living in AWS that is
*not* byte-identical to that canonical form is rewritten, even if a human
would call it "the same JSON." The tool has no notion of "close enough,"
because "close enough" is where non-determinism and endless drift live. A
byte comparison is the only comparison that gives a stable, repeatable answer
to "is this converged?"

## Convergence, and what "done" means

Convergence has a precise meaning here: a scope is converged when there are
zero operations left to execute *and* zero conflicts. "Unchanged" secrets do
not count against convergence — an unchanged secret is not an operation, it
is a fact recorded in the summary counts. A plan can report a hundred
unchanged secrets and still be fully converged.

This is why re-running a converged scope is a pure no-op, and why that
property is load-bearing rather than incidental. A `plan` against a converged
scope safely answers "has anything drifted?" without touching AWS, which is
exactly what you want gating a pull request. A `sync` against a converged
scope does nothing and costs almost nothing, which is exactly what you want
running on every merge. The tool is designed so that the steady state is
cheap and safe to re-enter as often as you like.

## Deletion is soft, reversible, and a two-step lifecycle

The tool never deletes a secret outright. Removing a committed file schedules
the corresponding secret for deletion within a recovery window, using AWS's
own scheduled-deletion mechanism; forced, immediate deletion is not something
the tool can do at all. Within that window the secret still exists, merely
marked for removal, and re-adding the file brings it back rather than
creating a fresh one.

That "bring it back" path is a genuine two-step lifecycle, and the reason is
worth understanding because it explains a plan you will otherwise find odd. A
secret scheduled for deletion has its value withheld — the tool does not read
the stored payload of a scheduled member, so it cannot know whether the value
still matches the commit. So restoration cannot be a single move. The tool
first cancels the scheduled deletion, which makes the secret active again but
possibly holding a stale value, and then performs at most one follow-up update
to write the committed value. Restore, then possibly one update — if the
recovered value already matches the commit byte for byte, the follow-up is a
no-op and no update is written. If you re-add a file
during its recovery window and see both a restore and an update attributed to
it, that is the model working as designed, not a redundant write. Past the
window the secret is truly gone, and re-adding the file is an ordinary create.

The recovery window is a safety net with a real duration, and choosing it,
and the procedure for intentionally retiring secrets, belong to [how to
decommission or empty a scope](../how-to/decommission-a-scope.md).

## Ordering is chosen for safety

When a single `sync` has several things to do, it does them in a fixed order:
restores first, then creates, then updates, and only then scheduled
deletions. This is not arbitrary and it is not merely for determinism, though
it is deterministic. The order is chosen so that a run which fails partway
through never leaves the scope worse than it found it. Restores and creates
bring secrets into existence, updates bring them current, and deletions —
the only destructive step — come dead last. If something goes wrong before
the tool reaches the deletion phase, nothing has been removed. New and
updated state lands before anything is scheduled to disappear, so a partial
run never takes access away before the replacement is in place. The exact
phase ordering, including how ties are broken, is stated in the
[reference](../reference/reconciliation.md).

## The empty-state gate, and why it exists

There is one place where the model deliberately refuses to follow its own
logic to the letter, and it is worth understanding as a concept even though
the procedure for working with it lives elsewhere. If the desired state is
*completely empty* and the plan that follows would schedule deletions, the
tool stops and refuses the run instead of quietly scheduling the whole scope
for deletion.

The reasoning is a judgement about probability. An empty desired set that
would delete everything is far more often an accident than an intention — a
mistyped source root, a checkout that landed in the wrong directory, a build
step that never produced the files — than it is a genuine, considered
decision to wipe a scope. The cost of being wrong is asymmetric: refusing a
legitimate teardown costs one explicit opt-in, while proceeding with an
accidental one costs an entire scope. So the tool treats total emptiness as
dangerous by default and requires the operator to say, explicitly, "yes, I
mean it." Note the narrowness of the guard: it triggers *only* when the
desired set is empty. Deleting some secrets out of a scope that still holds
others is ordinary reconciliation and is not gated at all. How to
intentionally cross that gate is covered in [how to decommission or empty a
scope](../how-to/decommission-a-scope.md).

## What a converged sync does, and does not, promise

It is easy to read "converged" as a stronger guarantee than it is, so it is
worth ending on its limits. A converged `sync` means that, at the moment the
tool observed and then independently verified the scope, AWS matched the
committed state. That is a real and useful guarantee. It is not a promise
that AWS is globally, instantaneously consistent, and it is not protection
against a second writer changing the same scope behind the tool's back.
Correctness rests on a single writer per scope and on AWS eventually
settling — assumptions that hold quietly most of the time and become visible
precisely when a run comes back inconclusive rather than converged.

Those failure-adjacent topics — why AWS's eventual consistency forces
independent verification, why some outcomes are inconclusive rather than
failed, why re-running is the recovery path, and why a single writer per
scope is non-negotiable — are a coherent subject of their own, treated in
[consistency, verification, and recovery](consistency-and-recovery.md). The
happy-path model on this page is what they build on: know how the tool
decides, and the ways it can be uncertain about whether a decision took hold
become far easier to reason about.

## Related reading

- [About ownership, scope, and fail-closed safety](ownership-and-scope.md) —
  what makes a secret "yours," and why changing scope inputs silently
  abandons secrets.
- [About consistency, verification, and recovery](consistency-and-recovery.md)
  — eventual consistency, inconclusive results, and why re-running is safe.
- [About the security and trust model](security-and-trust.md) — what stays
  confidential and why the binary's provenance is verified.
- [Reconciliation reference](../reference/reconciliation.md) — the exact
  decision matrix, naming grammar, phase order, and conflict classes.
- [How to decommission or empty a scope](../how-to/decommission-a-scope.md) —
  the procedure behind soft deletion and the empty-state gate.
- [Tutorial: your first reconciliation](../tutorial/first-reconciliation.md) —
  watch the model prove itself against a throwaway scope.

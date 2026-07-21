# About ownership, scope, and fail-closed safety

sops-aws-sync runs with your AWS identity, inside a namespace it almost never
has to itself. AWS Secrets Manager is a flat, account-wide store. Other tools,
other pipelines, and human operators all write to the same place. So before the
tool can safely create, update, or delete anything, it has to answer a question
that has nothing to do with reconciliation and everything to do with restraint:
*which of these secrets are mine to manage, and which must I leave alone?*

The answer is what this document is about. Ownership is the boundary the tool
draws around the secrets it created, and it draws that boundary deliberately
narrow. Everything outside it is refused, not adopted. That conservatism is the
single most important thing to internalize before you point the tool at a real
account, because the same mechanism that keeps it from touching someone else's
secrets is also the mechanism that will silently walk away from *your* secrets
if you change the wrong piece of configuration. Understanding one means
understanding the other.

## What makes a secret yours

Ownership is asserted with tags, not with names, paths, or guesswork. When the
tool creates a secret it stamps three reserved tags onto it, and from then on
those tags are the only evidence it trusts about whether the secret belongs to
it. A secret with a familiar-looking name but without the right tags is not
yours; it is a stranger that happens to share your naming convention.

The three reserved tags carry distinct jobs:

- A fixed **managed-by** marker (`sops-aws-sync:managed-by`, always the literal
  value `sops-aws-sync`) says "some instance of this tool created this."
- A **scope** digest (`sops-aws-sync:scope`) says "and it was *this*
  deployment."
- A **source** digest (`sops-aws-sync:source`) says "and it came from *this*
  committed file."

A secret counts as owned only when the managed-by marker **and** the scope
digest both match exactly. The source tag is not part of the ownership test; it
is a finer binding used within an owned scope, and it is discussed on its own
below. This two-part test is worth pausing on, because it is stricter than it
first appears: carrying the managed-by marker alone is not enough. A secret
created by a different deployment of the tool is just as foreign to your run as
a secret the tool never touched.

Ownership is never inferred and never granted retroactively. The tool will not
look at an untagged secret whose name matches a file you committed and decide to
"adopt" it. There is no adoption path at all. This is a deliberate choice over
the obvious alternative — treating name-prefix membership as ownership — because
a name is something anyone can create by accident or coincidence, whereas a tag
the tool wrote at creation time is a durable, first-party claim. Ownership by
tag means the tool can always tell the difference between a secret it is
responsible for and one that merely looks like it.

## Scope: the identity that ties a checkout to its secrets

The scope digest is the heart of ownership, so it is worth understanding what it
actually is. It is a single SHA-256 hash computed over three inputs: your
**repository identity**, your **source root** (the directory the tool reads
committed documents from), and your **secret prefix** (the namespace it writes
under in AWS). Those three values, combined under a versioned label, collapse
into one opaque fixed-length digest that becomes the scope tag.

Collapsing three human-readable settings into one hash is a design decision with
a purpose. The digest is a compact, tamper-evident fingerprint of "who is
allowed to manage secrets here." Because it is derived rather than configured,
two deployments cannot accidentally claim the same scope by choosing the same
tag string, and an operator cannot hand-forge a plausible scope value. The
inputs are combined unambiguously — a repository identity of `a/b` with root `c`
can never hash to the same scope as `a` with root `b/c` — so distinct
deployments always get distinct scopes even when their settings rhyme.

Scope is what makes it safe for many independent deployments to share one AWS
account. Each team, each repository, each environment computes its own scope
from its own settings, and each one is blind to every other. The tool's world is
not "all the secrets in the account"; it is "all the secrets carrying my exact
scope tag." Everything else, tagged or not, is outside its field of view.

The label folded into the digest carries a version (`v1`). That versioning is
not decorative: it is an escape hatch for the project to evolve the ownership
scheme in the future without silently colliding with secrets created under the
old scheme. It also means the scope is, in a sense, one more input that could
change — which is the thread that leads directly to the trap below.

## The re-scoping trap

Here is the concept that operators hit hardest and recover from least well.
**The three scope inputs are an identity, not a preference.** Changing the
repository identity, the source root, or the secret prefix does not
*re-configure* an existing deployment — it *mints a brand-new scope that owns
nothing you created before.* The old secrets keep their old scope tag. The new
scope's digest does not match them. As far as the reconfigured tool is
concerned, they were never its secrets at all.

What happens next depends on a subtlety worth spelling out, because it is where
the trap gets its teeth. The scope digest and the AWS secret name are computed
from overlapping but *different* inputs. The name is built from the secret
prefix and the file's path beneath the source root; it does not depend on the
repository identity. So a scope change lands in one of two very different ways.

When the change also alters the derived names — most obviously, changing the
**secret prefix** — the new deployment computes an entirely fresh set of names.
None of them exist in AWS yet, so the tool cheerfully *creates a whole new set
of secrets* under the new prefix. Meanwhile the old secrets, under their old
names and old scope, are invisible to the new scope. They are not deleted. They
are not flagged. They simply sit there — still costing money, still holding live
secret material, still readable by anyone with access — no longer reconciled by
anything. This is silent abandonment, and nothing about the run reports it as a
problem. The plan looks like a clean batch of creates; the exit code is success.

When the change leaves the derived names unchanged — for example, changing only
the **repository identity** — every desired name now resolves to a secret that
*already exists* in AWS but carries the old scope tag. The tool sees a secret
bearing the managed-by marker but the wrong scope, classifies it as foreign, and
refuses it. Because a single foreign secret blocks the entire plan (the
fail-closed rule described below), the run stops before any mutation with an
ownership conflict. This outcome is loud and safe: nothing is abandoned and
nothing is overwritten, but nothing happens either until you resolve it.

Both outcomes share the same root cause and the same lesson. The tool never
adopts, retags, or migrates the prior secrets for you. There is no "rename my
scope" operation, because from the tool's point of view a scope has no
continuity across a change of its inputs — the old and new scopes are simply two
different owners. The only correct way to treat a scope change is as a
**migration**: you are moving secrets from one owner to another, and you must
deliberately create them under the new scope and retire the old set yourself
(see [How to decommission or empty a scope
safely](../how-to/decommission-a-scope.md)). If you reach an ownership conflict
instead, [How to diagnose and recover from a failed
run](../how-to/diagnose-and-recover.md) covers turning it back into a working
state.

The practical guidance is blunt: **choose your repository identity, source root,
and secret prefix once, early, and freeze them.** Treat them the way you would
treat a database name or a Kafka topic — a stable coordinate that other things
depend on, not a knob to tune. A stable scope is precisely what lets a Git
checkout and its AWS-side secrets stay bound to each other over months and
across many runs. A casually edited scope input severs that binding in a way the
tool is designed not to fight, because fighting it would mean guessing, and
guessing about ownership is the one thing it refuses to do.

## Source identity: a finer binding inside the scope

Ownership answers "is this secret in my scope?" The source tag answers a
narrower question inside an owned scope: "did this secret come from *this exact
committed file?*" It is a hash of the file's normalized, repository-relative
path, recorded at creation time alongside the scope.

The word "file" there is deliberately logical, not literal — a distinction that
matters now that a source document can be committed as JSON or YAML. Before it is
hashed, the path's encoding suffix is folded to one canonical form, so
`db.sops.json`, `db.sops.yaml`, and `db.sops.yml` sitting at the same location
all produce the identical source digest. The derived secret name strips the
encoding suffix too, so those three also resolve to the identical name.
Re-encoding a document in place — rewriting a JSON source as YAML, say — therefore
changes neither the name nor the source tag: ownership carries straight over and
the tool keeps reconciling the secret exactly where it stood. That is safe
because both encodings decrypt to the *same* canonical value, so an encoding-only
change is a true no-op — there is nothing to update and no drift to detect.

What the source identity still tracks is the document's logical position: its
stem and its directory beneath the source root. Change either — rename the stem,
or move the document into a different subdirectory — and the hashed path changes,
so the tool sees a genuinely different source. That is the intended behavior: a
document at a new location is a new source of truth, and only its encoding is
treated as an interchangeable detail.

Its job is to stop one file from silently commandeering a name that another file
established. Suppose a secret was created from `secrets/db/prod.sops.json` and
carries that path's source digest. Later, a *different* committed file comes to
resolve to the same AWS name. The name matches, the scope matches — but the
source digest does not. Rather than overwriting the existing secret with an
unrelated file's contents, the tool treats the mismatch as a foreign secret and
refuses it. The source tag turns what could have been a silent, destructive
overwrite into a visible standoff.

A related collision is caught even earlier: if two committed files in the same
revision would map to the same AWS name, the plan is rejected outright rather
than letting one arbitrarily win. Between the two mechanisms, no name can ever be
quietly reassigned from one source of truth to another — every AWS secret stays
bound to the one committed file that owns it.

## Write-once tags, and why there is no repair path

The reserved tags are written exactly once, at the moment the secret is created,
and never afterward. Updating a secret's value does not rewrite its tags;
restoring it does not rewrite its tags; there is no separate "retag" step
anywhere in the tool. Ownership metadata is set at birth and then treated as
immutable.

This has a sharp consequence. If the reserved tags on a secret are altered or
removed out of band — someone edits them in the console, an infrastructure tool
strips them, a policy rewrites them — the tool has no way to notice and no way to
repair them. From that point the secret is orphaned from the tool's view: it no
longer matches the scope, so it is either invisible (if discovery never sees it)
or foreign (if a desired name collides with it), and the only remedies are
manual re-tagging or letting the tool create a fresh secret. There is no
self-healing.

The flip side of write-once is restraint in the other direction: the tool reads
and cares about *only* those three reserved keys. Any other tags you or your
organization attach to a managed secret — cost-center labels, compliance
markers, anything — are neither consulted when the tool makes decisions nor
disturbed when it writes. The tool owns its three tags and ignores everything
else on the resource. Write-once ownership metadata plus hands-off treatment of
your tags keeps the ownership signal simple and trustworthy: it is exactly what
the tool wrote at creation, nothing more and nothing less.

## Fail-closed: refuse the whole plan rather than guess

Ownership decides what the tool *may* touch. Fail-closed decides what it does
when something it cannot safely touch shows up: it stops, entirely, before
changing anything.

A plan is the union of the secrets you have committed and the secrets discovered
under your scope. If *any* member of that union is something the tool must not
act on — a foreign secret, a name collision, or an owned secret in an unsafe
state — the whole plan is a conflict and **no mutation happens at all.** The tool
does not apply the safe operations and skip the problematic one. It does not do
its best and report the rest. One conflict anywhere aborts the run before the
first AWS write, and convergence is defined to require both zero pending
operations *and* zero conflicts. There is no partial success.

Crucially, fail-closed governs owned secrets too, not just strangers. A secret
can carry all the right tags and *still* be refused because it is in a state the
tool considers unsafe to reconcile — for instance one that is service-owned,
has rotation enabled or in progress, is replicated to other Regions, or has an
ambiguous notion of its current version. Ownership is necessary for the tool to
act, but it is not sufficient; the secret also has to be in a shape the tool can
reason about deterministically. The specific conditions that trigger each
refusal, and what to do about them, are catalogued in the [reconciliation
reference](../reference/reconciliation.md) and worked through in
[diagnose-and-recover](../how-to/diagnose-and-recover.md); the point here is the
philosophy behind them.

Why refuse the whole batch instead of doing the safe part? Because of what the
tool is. It runs with the caller's AWS identity, which means its blast radius is
whatever that identity can reach. In that setting, a best-effort partial apply
is more dangerous than doing nothing: it leaves the account in a state that is
neither the old one nor the intended new one, it makes the failure harder to
reason about, and it may have already deleted or overwritten something on the
strength of an assumption that later turned out wrong. Refusal, by contrast,
leaves everything exactly as it was and hands the decision to a human, who has
context the tool does not. The tool never adopts an unowned secret, never
overwrites a stranger, never retags to force ownership, and never deletes or
restores something it is unsure about. When in doubt, it declines. This is the
same instinct as the empty-desired-state guard and the soft, reversible deletion
model — described in [the reconciliation
model](reconciliation-model.md) — applied to the ownership boundary: the tool
is built to make the *unsafe* thing the thing that does not happen.

## What this means for how you operate

If you take one operational commitment from this document, make it this: decide
your scope inputs deliberately and then stop changing them. The repository
identity, source root, and secret prefix are the coordinate system your secrets
live in. Everything downstream — which secrets are yours, which run may manage
them, whether a re-run is a safe no-op or an accidental migration — is anchored
to that coordinate staying still.

Everything else follows from taking ownership seriously as a boundary you opt
each secret into exactly once, at creation, and never renegotiate. The tool's
refusals are not obstacles to work around; they are the boundary doing its job.
When it declines to touch a secret, the right response is almost never to force
it — it is to understand why the secret fell outside the boundary, and to fix
that, in AWS or in your configuration, before asking the tool to try again.

## Common misconceptions

### "The tool owns everything under my prefix"

It does not. Owning a name prefix in AWS is not the same as owning the secrets
that happen to sit under it. A secret is yours only if the tool created it and
tagged it with your scope. A pre-existing secret whose name falls under your
prefix, or one created by a different deployment, is foreign — and a foreign
secret at a name you want will block the run, not be quietly absorbed into it.

### "Reorganizing my repo or renaming things is a harmless refactor"

It is not, if the thing you rename is a scope input. Moving the source
directory, renaming the repository identity, or changing the secret prefix
re-scopes ownership. Depending on whether the derived names change with it, you
either abandon your existing secrets silently or hit a wall of ownership
conflicts. Either way it is a migration, and it needs to be planned as one.

### "If a tag gets messed up, the tool will fix it on the next run"

It will not. Reserved tags are written once at creation and never repaired.
Altered or missing ownership tags orphan a secret permanently from the tool's
view until you re-tag it by hand or let the tool create a replacement. There is
no reconciliation of the tags themselves.

## Related concepts

- [About the reconciliation model](reconciliation-model.md) — how owned
  secrets are turned into create, update, restore, and deletion decisions, and
  why deletion is soft and reversible.
- [About consistency, verification, and recovery](consistency-and-recovery.md)
  — why a single active writer per scope matters and how the tool behaves under
  failure.
- [About the security and trust model](security-and-trust.md) — how secret
  names are kept out of logs, why AWS authentication is the caller's
  responsibility, and what running with your identity implies.
- [Reconciliation reference](../reference/reconciliation.md) — the exact tag
  keys and derivations, the conflict-class catalog, and the AWS operations the
  tool issues.

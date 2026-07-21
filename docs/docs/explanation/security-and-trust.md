# About the security and trust model

sops-aws-sync runs with your AWS identity and handles your decrypted
secrets. Those two facts are the whole security story. The binary you
install will hold your credentials and your plaintext at the same time,
so "can I trust this binary?" and "what actually stays confidential?"
are not two separate questions — they are the same question asked from
two directions. This document explains both boundaries: what the tool
keeps confidential (and, just as importantly, what it does not), and how
you can trust that the binary doing the handling is the one the project
actually published.

Read this before you grant the tool AWS access. Everything here is about
understanding the model well enough to reason about your own exposure;
the procedures that act on it live in the how-to guides linked
throughout.

## What is actually secret

The most dangerous assumption an operator can bring to this tool is that
everything inside a committed SOPS document is secret. It is not, and
acting as though it is will eventually leak something you believed was
protected.

SOPS encrypts *values*, not whole documents. It supports partial
encryption, and the tool accepts that faithfully: it delegates
decryption wholesale to SOPS and never asserts that every leaf was
encrypted. Whatever your encryption policy left in cleartext stays in
cleartext in the committed file. In practice, a committed SOPS document
— a `.sops.json`, `.sops.yaml`, or `.sops.yml` file — exposes, to
anyone who can read the repository:

- the document structure itself — every key and member name;
- any leaf your policy deliberately left unencrypted;
- the SOPS metadata block, which can name key backends and identifiers
  such as a KMS ARN or an age recipient;
- the file's path in the repository, which the tool also derives the
  secret's name from.

Only the encrypted values are confidential, and even those are
confidential in Git only to the extent your repository's access controls
make them so. The tool's confidentiality guarantees begin at the point
of decryption and apply to what the tool does with the plaintext; they
say nothing about what SOPS chose to leave visible in the file you
committed. Treat the cleartext portions of a SOPS document as public to
everyone with repository access, and design your key layout and
repository permissions on that basis.

There is one integrity guarantee that does travel with the file. SOPS
verifies the document's MAC as part of decryption, over the exact
committed bytes. Tampered ciphertext fails to decrypt, and because the
tool reads, decrypts, and validates the entire desired state before it
issues a single AWS mutation, a tampered or corrupt document stops the
run before anything is written.

## How secrets are kept from leaking

Once a value is decrypted, it lives only in process memory. It is never
written to the machine report, never placed on a command line, never put
into an environment variable, and never logged. What the tool does *not*
promise is to scrub that memory: Go's garbage-collected strings and byte
slices, and the AWS SDK's own buffers, give no reliable way to erase
plaintext from RAM while the process is alive. The guarantee is about
egress — the channels through which a secret could escape — not about
zeroing memory mid-run. For a long-lived host this is a threat-model
boundary worth stating plainly; for the short-lived CI process the tool
is designed to run in, it is a small residual.

The safety of those egress channels does not rest on a scrubber that
hopes to catch every sensitive string on its way out. It is structural:
sensitive text never enters the tool's error and log values in the first
place.

- AWS failures are reduced to a typed, safe error that carries only an
  operation name, a modeled error code, a fault class, and an AWS
  request id — never the raw SDK message, which can contain ARNs. The
  request id is the value you hand to AWS support; the rest stays out.
- A decryption failure — wrong key, missing key, corrupt ciphertext, a
  bad MAC, or an unreachable KMS or age backend — collapses to a single
  fixed message. The underlying reason, which could name a key or a
  provider, is discarded rather than surfaced.
- SOPS's own library logging, which can print provider identifiers such
  as a KMS ARN, is silenced entirely, independent of your log settings.
- Operations log a run-local ordinal such as `resource-0001` in place of
  the secret's name. Real names appear only if you deliberately opt in
  with `--show-resource-names`, and only then in a log sink you trust.
- The machine report is an exact-key allowlist. It carries counts,
  statuses, versions, and durations; a field that is not on the list is
  rejected as an error rather than passed through silently.

None of this weakens at `--log-level=debug`. Debug logging adds detail
about decisions and retries; it never adds plaintext. That is why the
default logs and the report are safe to archive, attach to a ticket, or
paste into a chat without a redaction pass of your own — a property the
GitHub Action leans on when it forwards CLI output and publishes a
summary.

It is worth being explicit about why the tool prefers this structural
approach over a redaction filter, because it is a deliberate stance. A
best-effort redactor is a denylist: it has to anticipate every shape of
sensitive data and every place it might appear, and it fails *open* the
first time it misses one. An allowlist fails *closed*: only the values
explicitly permitted can appear, so a sensitive field added to the code
tomorrow is excluded by default rather than by the redactor's luck. When
the cost of a miss is a leaked secret, failing closed is the only
defensible default. (The exact report fields and log vocabulary are
catalogued in the [results](../reference/results.md) and
[configuration](../reference/configuration.md) references; this section
explains why they are shaped the way they are, not what each field is.)

## Authentication is yours to establish

Neither the CLI nor the Action accepts credential material. There is no
access-key flag, no secret-key flag, and no session-token input anywhere
on the surface. Credentials come exclusively from the ambient AWS SDK
chain that you set up before the tool runs — environment variables,
shared config and profiles, SSO, IRSA, or instance and container roles.
The only steering the tool offers is a region and a profile, and the
Action forwards only a region.

The optional `github-token` is a key for an entirely different lock. It
authenticates GitHub release metadata, asset download, and attestation
verification — and nothing else. It is masked the instant it is read,
handed to the GitHub CLI only inside an isolated configuration directory
with the ambient GitHub-token environment variables stripped out first,
and it is never forwarded to the CLI or to AWS. Its default is the
workflow's own automatic token; you override it only for private or
cross-repo installs.

The rule that falls out of this boundary is short and non-negotiable:
never hand AWS credentials to a run you do not trust. In particular,
never expose them to a `pull_request`-triggered workflow that carries
code you have not reviewed. The tool executes with exactly the identity
you give it and no more, which is why the strength and scope of that
identity is a decision you own — and why the provenance of the binary
consuming it is the next thing to get right. (Wiring the credential step,
its OIDC permissions, and the single-writer concurrency it depends on
is covered in [deploy with GitHub
Actions](../how-to/deploy-with-github-actions.md); the least-privilege
policy itself is in [grant AWS
access](../how-to/grant-aws-access.md).)

## Trusting the binary: what "attested" proves

Because the binary you install will run with your AWS identity and your
plaintext, "where did this binary come from?" is a security question,
not a convenience. The Action answers it before it executes anything, by
requiring two independent proofs.

First, the downloaded binary's SHA-256 must match its entry in the
release's `checksums.txt`. Second — and this is the load-bearing one —
`gh attestation verify` must confirm a signed SLSA provenance statement
against a policy the Action pins in its own code: the artifact must have
been built by this repository, by its own `attest.yml` signer workflow,
for the exact release tag and its exact commit, with the GitHub Actions
OIDC issuer as the certificate authority, as SLSA provenance v1, and on
a GitHub-hosted rather than a self-hosted runner. A checksum on its own
only proves a file matches a manifest; it says nothing about who
produced the manifest. Provenance supplies the missing "who."

It is worth being precise about what each proof binds, because both bind
the binary directly. The release build stamps every entry in
`checksums.txt` as its own provenance subject — `actions/attest` with
`subject-checksums: checksums.txt` expands each line into a subject
pairing an asset name with its SHA-256, so each individual binary, not
the manifest, is what the attestation covers. The Action therefore checks
the binary twice against the same digest: a plain SHA-256 equality of the
downloaded bytes against the manifest line for your asset, and a
provenance check that runs `gh attestation verify` on the binary itself
and requires a signed statement whose subject digest equals that same
SHA-256. There is no hash-of-hashes indirection and no transitive trust
through the manifest's own provenance; the checksum step proves the bytes
match the manifest, and the provenance step proves those exact bytes were
built by the pinned workflow. The Action re-runs both checks even on a
tool-cache hit, so a cache entry poisoned between jobs on a shared runner
cannot be trusted merely because it already exists. Any failure is
terminal: the Action fails closed with a deliberately generic message,
and the binary never runs. (The exact command and its flags are in
[verify a release
binary](../how-to/verify-a-release-binary.md); you only run that by hand
when you install the CLI directly, since the Action does it for you.)

This is also why a release built by a fork, a re-signed binary, or an
artifact from a moved or deleted tag cannot pass: none of them satisfies
the pinned identity, ref, and issuer, and that is by design.

It is equally important to be clear about the limits of what an
attestation proves. It establishes build identity and integrity — that
this exact artifact was produced by this project's workflow for this
commit. It does not prove the binary is free of bugs, it does not vouch
for how the code behaves at runtime, and it does not attest the upstream
dependency supply chain that fed the build. Provenance answers "is this
the artifact the project published?", not "is the project's code
correct?" Trusting the source is a prerequisite for, not a substitute
for, your own judgement about the tool.

## Version pinning as a trust control

Pinning is usually framed as a compatibility concern. Here it is also a
security control. The CLI version is fully pinned and immutable — an
exact semantic version, with no leading `v`, no ranges, and no "latest".
The CLI and the Action move in lockstep: a given Action release embeds
and installs exactly one matching CLI version, and the release process
refuses to let the two drift apart.

Two things follow for operators. Pin the Action to the full commit SHA
behind a release tag, not to a floating tag such as `@v1` or `@master`.
A floating tag is a mutable pointer; a future release, or a compromise
of the repository, can move it under you, and moving it is exactly the
attack a SHA pin removes — the pinned commit either is or is not the one
you reviewed. And because only the latest published release is supported
— there are no backports — staying current is the security-maintenance
path, not an optional upgrade.

Overriding `cli-version` away from the Action's embedded default is a
deliberate act with a consequence worth understanding: it couples you to
the report-schema and exit-status contract of that specific CLI, and a
mismatch surfaces as a hard "CLI exit status and redacted report
disagree" failure rather than a quietly wrong result. That fail-loud behavior is itself a
trust property — the fuller treatment lives in the
[results](../reference/results.md) reference.

## What this model does not give you

A security model is defined as much by the boundaries it declines to
cover as by the guarantees it makes. Three are worth stating outright,
because assuming otherwise creates real exposure.

It does not manage secrets that carry rotation, replication, or service
ownership. Rather than contend with another controller for a secret, the
tool refuses such secrets as conflicts and touches nothing. The reasoning
behind refusing rather than best-effort adopting is in [ownership and
scope](ownership-and-scope.md); the conditions that trigger each refusal
are catalogued in the [reconciliation](../reference/reconciliation.md)
reference.

It cannot hide the secret names or repository paths your Git history
already exposes. Structural redaction keeps those out of *this tool's*
logs and report, but the committed repository is its own disclosure
surface, governed entirely by your repository policy — not by anything
the tool can do after the fact.

And it is not an AWS authenticator. It consumes an identity you
establish and never mints or scopes one for you, so the blast radius of
a run is precisely the blast radius of the credentials you handed it.
Designing that identity for least privilege is your responsibility, and
the starting point is [grant AWS
access](../how-to/grant-aws-access.md).

## Common misconceptions

### "Everything in a SOPS document is confidential"

Only the encrypted values are. Keys, structure, unencrypted leaves, SOPS
metadata, and the file path are cleartext in Git and are only as private
as your repository. This is the assumption most likely to leak
something.

### "An attested binary is a safe binary"

Attestation proves the artifact's origin and integrity, not its
correctness or its runtime behavior, and not the supply chain of its
dependencies. It tells you the binary is genuinely the project's; it does
not tell you the project's code is flawless.

### "Redaction means I still have to scrub the logs and report myself"

The default logs and the report are safe by construction — an allowlist,
not a best-effort filter — so they can be archived or shared as-is.
Enabling `--show-resource-names` is the one deliberate exception, and
only you can decide whether a given log sink is trusted enough for it.

### "Pinning the Action to `@v1` is good enough"

A floating tag can be moved; a full commit SHA cannot. Pin to the SHA
behind a release, and upgrade forward, since only the latest release is
supported.

## Related reading

- [About consistency, verification, and
  recovery](consistency-and-recovery.md) — why a single writer per scope
  is a correctness requirement, not just an operational nicety, and why
  runs can end inconclusive.
- [About ownership, scope, and fail-closed
  safety](ownership-and-scope.md) — why the tool refuses unowned and
  unsupported secrets instead of adopting them.
- [How to verify a downloaded release
  binary](../how-to/verify-a-release-binary.md) — the manual checksum
  and attestation procedure for direct CLI installs.
- [How to deploy with GitHub
  Actions](../how-to/deploy-with-github-actions.md) — putting the
  credential boundary and version pinning into a real workflow.
- [How to grant least-privilege AWS
  access](../how-to/grant-aws-access.md) — designing the identity the
  tool runs as.

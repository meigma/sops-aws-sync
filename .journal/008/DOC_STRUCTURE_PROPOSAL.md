
# sops-aws-sync operator documentation — final structure proposal

A single, Diátaxis-strict documentation set for operators who **deploy and administrate** sops-aws-sync. It replaces the defunct `docs/` tree wholesale.

---

## 1. Design principles

These rules govern every decision below. They are the arbiter when two placements compete.

1. **One document, one Diátaxis type — no exceptions.** Every file is exactly one of tutorial / how-to / reference / explanation. Adjacent-type "blur" is actively policed by the type-boundary rules in §4 and by a per-document *deliberate exclusions* list that names the banned content **and the document that owns it**.

2. **The explanation spine is load-bearing; everything else leans on it by link.** This tool's entire value is its careful, non-obvious semantics, and the user's binding goal is that operators leave with a correct mental model. Four explanations carry the "why." How-tos, references, and the tutorial *point* at them and never re-teach.

3. **All enumerable truth lives in reference, and only in reference.** Exit codes, statuses, verification values, the report schema, the decision matrix, conflict classes, every flag/env/input, naming rules — each is catalogued exactly once, in a dry reference. Explanations narrate these *conceptually* ("the four verification outcomes"), never reproduce the table. This is the single cleanest boundary the judges identified and it is the spine of the set's purity.

4. **All procedure lives in how-to, and only in how-to.** Steps, variations, escape routes. How-tos assume competence and link out for both facts and reasoning.

5. **A smaller set of mature documents beats many shallow ones.** Quadrants are deliberately unequal (Diátaxis permits this): explanation and how-to carry the weight because operators are here to *understand* and to *do*. There is exactly **one** tutorial. No document is a stub; each earns its place with tool-specific substance.

6. **Every document must clear the "an-AI-prompt-could-answer-this" bar.** Generic SOPS onboarding, `gh` installation, and vanilla AWS credential/OIDC setup are excluded and linked out (§5). What remains is the substance unique to *this* tool.

7. **Titles signal type.** Explanations use "About …". How-tos use "How to …". References use "… reference". The tutorial uses "Tutorial: …". A reader knows the type before the first line.

---

## 2. The proposed tree

```
docs/
├── index.md                                 # landing — orient in one screen + route by operator moment
├── tutorial/
│   └── first-reconciliation.md              # tutorial — plan → sync → verify loop in a throwaway scope
├── how-to/
│   ├── deploy-with-github-actions.md        # how-to — production workflow: OIDC, perms, single-writer, pin, plan/sync, drift-gate, token, self-hosted
│   ├── grant-aws-access.md                  # how-to — least-privilege Secrets Manager + KMS policy and credentials
│   ├── verify-a-release-binary.md           # how-to — manual checksum + SLSA attestation for direct (non-Action) CLI use
│   ├── decommission-a-scope.md              # how-to — retire secrets, the empty-state gate, recovery window, restore-in-window
│   └── diagnose-and-recover.md              # how-to — symptom → remedy for exit 3/4/5/6/130 and Action failure messages
├── explanation/
│   ├── reconciliation-model.md              # explanation — git-as-truth, the decision, convergence, soft-delete lifecycle, determinism
│   ├── ownership-and-scope.md               # explanation — reserved tags, scope digest, the re-scoping trap, fail-closed conflicts
│   ├── consistency-and-recovery.md          # explanation — eventual consistency, verification, ambiguous writes, no-rollback, interruption
│   └── security-and-trust.md                # explanation — partial confidentiality, structural redaction, supply-chain provenance, version pinning
└── reference/
    ├── configuration.md                     # reference — every CLI flag / env var / Action input & output, precedence, streams
    ├── reconciliation.md                    # reference — source format, naming, tags, decision matrix, phase order, conflict classes, AWS ops
    └── results.md                           # reference — exit codes, statuses, verification values, report v1 schema, Action-message map
```

**Shape:** 1 landing + 1 tutorial + 5 how-to + 4 explanation + 3 reference = **13 content documents**.

**Why 13 and not 12.** The one document that separates this set from the leanest rival is the standalone `ownership-and-scope` explanation. The map names silent scope abandonment "operationally severe" and it is the single hazard an operator is most likely to hit and least able to recover from. Folding it into `reconciliation-model` would bury the product's #1 trap as one bullet in a fourteen-bullet mega-explanation — an economy win that quietly loses the user's most important constraint (a correct mental model) and violates Diátaxis's "topic-bounded, coherent" rule for explanation. "Reconcile my secrets toward Git" and "decide what is *mine* to touch and refuse everything else" are two coherent topics with a natural conceptual seam. Keeping them apart is a decisive choice, not an oversight. Economy is instead recovered *inside* the reference quadrant (a 3-doc triad, not 4, with no scattered facts) and by folding IAM into a how-to rather than a fourth reference.

---

## 3. Document specifications

Reading conventions for this section: **Type** is the Diátaxis quadrant. **Reader moment** is the situation the reader is in. **Excludes** names banned adjacent content *and its correct home*.

---

### 3.1 `docs/index.md` — sops-aws-sync operator documentation

- **Type:** Landing (navigational; deliberately not one of the four types — carries no teaching, facts, or procedures of its own).
- **Purpose:** Orient a new operator in one screen and route them to the one document that serves their current need.
- **Reader moment:** Just arrived at the docs, or returning to find a specific answer, unsure which page covers their situation.
- **Content outline:**
  - One paragraph on what the tool is: a Go CLI plus a paired, supply-chain-verified Node 24 GitHub Action that reconciles committed `*.sops.json` documents at one exact Git commit into an explicitly owned namespace in AWS Secrets Manager (create / update / restore / scheduled-delete). `plan` is read-only; `sync` mutates and always verifies.
  - The single load-bearing sentence of the model, linked to `reconciliation-model`: the committed Git revision is the desired state; you change AWS by committing files, never by editing secrets directly.
  - **"Read before you run `sync`"** — the five highest-stakes invariants as one-line *pointers that link out and are not explained here*: commit-as-source-of-truth; changing repository-id / source-root / secret-prefix silently orphans prior secrets (→ `ownership-and-scope`); the empty-desired-state deletion gate (→ `decommission-a-scope`); the single-writer-per-scope requirement (→ `consistency-and-recovery`); partial-encryption confidentiality — not everything in a `.sops.json` is secret (→ `security-and-trust`).
  - A route-by-moment map: *understand how it works* → the four explanations (the backbone, read in order); *learn by doing* → the tutorial; *deploy in CI* → `deploy-with-github-actions` + `grant-aws-access`; *run the CLI directly* → `verify-a-release-binary` + the tutorial; *look something up* → the three references; *a run failed* → `diagnose-and-recover`; *remove secrets* → `decommission-a-scope`.
  - A compact **"what this tool is NOT"** as pointers: not a secret editor; not an AWS authenticator (bring SDK-chain credentials); refuses secrets under rotation, replication, or service ownership; no force delete; JSON-only `*.sops.json`; Linux/macOS on amd64/arm64 only.
  - Pointers out: the repo `README.md` for the quick-start Action snippet and build-from-source; `SECURITY.md` for private vulnerability reporting.
  - Audience note: these docs serve operators; contributor and release-pipeline internals are out of scope.
- **Excludes:** conceptual teaching → the explanations; procedures → the how-tos; flag/exit-code/schema tables → the references; the README's quickstart pitch → `README.md` (the landing routes, it does not duplicate).
- **Cross-links:** all four explanations; the tutorial; `deploy-with-github-actions`; `diagnose-and-recover`; `configuration`; `results`; `../README.md`.

---

### 3.2 `docs/tutorial/first-reconciliation.md` — Tutorial: Your first reconciliation

- **Type:** Tutorial.
- **Purpose:** Teach the plan → sync → verify loop and let the operator *see* the reconciliation model prove itself — convergence, no-op re-runs, drift repair, soft-delete and restore — against a disposable scope.
- **Reader moment:** A newcomer with a throwaway AWS account/prefix and a working SOPS key who wants to build confidence by doing it once, safely.
- **Content outline (single linear path; every "why" is a link out, never inline teaching; exact expected output at each step):**
  - Show the destination first: by the end we will have created secrets from committed files, watched a re-run do nothing, repaired external drift, then soft-deleted and restored a secret — all inside a disposable `--secret-prefix` like `/scratch/demo`.
  - Prerequisites (linked, not taught): a local git checkout; two files we can already decrypt (link the SOPS project — do not teach SOPS); throwaway credentials resolvable via the standard AWS SDK chain for one region (link `grant-aws-access` for the real policy); the verified CLI installed (link `verify-a-release-binary`).
  - Step 1 — commit `secrets/alpha.sops.json` **and** `secrets/beta.sops.json`, each a single top-level JSON object. (Two files so the scope stays non-empty when we later delete one — this keeps the tutorial on one option-free path and never touches `--allow-empty`.) Note in passing: the tool reads the committed blob at `--revision`, not the working tree.
  - Step 2 — run `sops-aws-sync plan` with `--repository . --revision HEAD --repository-id you/demo --source-root secrets --secret-prefix /scratch/demo --aws-region …`; expected: JSON logs on stdout and `create=2`; notice resources appear as `resource-0001` ordinals, not real names.
  - Step 3 — add `--report-file plan.json` and open it; expected: a JSON report whose status and counts are visible while the value, name, and source path are absent; note it is safe to paste anywhere.
  - Step 4 — run `sops-aws-sync sync`; expected: two creates, then `verification=converged`, `status=converged`, exit 0; note `sync` always verifies — there is no apply-without-verify.
  - Step 5 — run `sync` again unchanged; expected `unchanged=2`, zero mutations — the pure no-op.
  - Step 6 — change one secret's value directly in the AWS console (guided), re-run `sync`; expected `update=1`; note the commit did not change — drift was repaired because the ownership tags made this secret "ours."
  - Step 7 — `git rm secrets/alpha.sops.json`, commit, `sync`; expected `schedule-delete=1` (the scope still holds `beta`, so no `--allow-empty` is needed); note the recovery window keeps it restorable.
  - Step 8 — re-add `alpha`, commit, `sync`; expected a restore-plus-update in one run.
  - What we learned: Git is the desired state; `plan` is always safe; `sync` mutates then verifies; re-runs are no-ops; drift is repaired; deletion is soft and reversible.
  - Next steps: `reconciliation-model` (why); `deploy-with-github-actions` (real use); `decommission-a-scope` (intentional teardown, including emptying a scope with `--allow-empty`).
- **Excludes:** any conceptual deep-dive (scope-digest math, eventual consistency) → the explanations; `--allow-empty` and full emptying of a scope → `decommission-a-scope`; alternative flags/config-file → `configuration`; failure branches → `diagnose-and-recover` (a tutorial must not fail). Requires active maintenance because Step 6 depends on the AWS console UI.
- **Cross-links:** `reconciliation-model`; `ownership-and-scope`; `verify-a-release-binary`; `deploy-with-github-actions`; `decommission-a-scope`; `configuration`.

---

### 3.3 `docs/how-to/deploy-with-github-actions.md` — How to deploy sops-aws-sync with GitHub Actions

- **Type:** How-to.
- **Purpose:** Wire the Action into a workflow that plans on pull requests and syncs on protected merges, with AWS OIDC auth, single-writer concurrency, and pinned, verified installation.
- **Reader moment:** An operator competent with GitHub Actions and AWS building the CI/CD pipeline that will run reconciliation against a real account — wanting production-safe wiring, not a toy example.
- **Content outline (directions + concise variations; links out for all facts and reasoning):**
  - Prerequisites (linked): an AWS role assumable via GitHub OIDC carrying the policy from `grant-aws-access`; SOPS decryption keys reachable from the runner; committed `.sops.json` files laid out per `reconciliation`; a chosen, frozen scope per `ownership-and-scope`; the full release commit SHA to pin.
  - Set workflow permissions: top-level `permissions: {}`, then per-job `contents: read` always, `id-token: write` only when AWS auth uses OIDC, `attestations: read` for private same-repo attestation verification.
  - Add the AWS credential step **first** (`aws-actions/configure-aws-credentials` via OIDC): the Action performs no AWS auth of its own and forwards only `--aws-region`, so missing/insufficient credentials surface later as opaque exit 4/5, not as an input error (link `diagnose-and-recover`).
  - Check out with `fetch-depth: 0` and `persist-credentials: false` so the committed blobs at the resolved revision are present.
  - Pin `meigma/sops-aws-sync` to the full release commit SHA (not `@v1` or `@master`) with a version comment; do not float `cli-version` (link `security-and-trust` for why).
  - Enforce single-writer correctness: a `concurrency` group unique per ownership scope with `cancel-in-progress: false`; run `sync` only from protected default-branch merges or a protected-environment dispatch — never from `pull_request` code holding AWS credentials.
  - **Variation A — plan on PRs with drift gating:** `mode: plan` + `fail-on-drift: true` (valid only in plan mode) → drift returns exit 2 and fails the check, gating merges. Booleans must be exactly `true`/`false`.
  - **Variation B — sync on merge:** `mode: sync`; do **not** set `fail-on-drift` (rejected in sync mode); gate downstream steps on the `status` output.
  - **Variation C — private or cross-repo consumers:** supply `github-token` scoped to `contents: read` and `attestations: read`; the default `${{ github.token }}` suffices for public same-repo installs; the token authenticates release download and attestation verify only, never AWS.
  - **Variation D — self-hosted / container runners:** node24 runtime, Linux/macOS on X64/ARM64, a `gh` new enough for the attestation policy flags, and the required `RUNNER_*`/`GITHUB_*` env present.
  - Consume results: read `status`, the per-operation counts, `verification-status`, `redacted-report-path`, and the job summary; optionally upload the redacted report as an artifact. Because raw stderr is suppressed to a fixed warning, debug via the JSON stdout logs and the report.
  - Verification: a clean run reports `status: converged` and `verification-status: converged`; anything else → `diagnose-and-recover`.
- **Excludes:** the full input/output and validation tables → `configuration`; the IAM/KMS policy itself → `grant-aws-access`; *why* provenance verification and single-writer matter → `security-and-trust` and `consistency-and-recovery`; exit-code/status meanings → `results`; generic "set up GitHub↔AWS OIDC" and "install `gh`" → linked upstream, not taught; manual binary verification (automatic here) → `verify-a-release-binary`.
- **Cross-links:** `grant-aws-access`; `configuration`; `results`; `security-and-trust`; `consistency-and-recovery`; `ownership-and-scope`; `diagnose-and-recover`; `verify-a-release-binary`.

---

### 3.4 `docs/how-to/grant-aws-access.md` — How to grant sops-aws-sync least-privilege AWS access

- **Type:** How-to.
- **Purpose:** Provision the exact prefix-scoped Secrets Manager and KMS permissions the tool needs, and establish credentials it can consume.
- **Reader moment:** An operator or cloud admin who knows IAM, setting up AWS before the first `plan`/`sync`, wanting the correct minimal policy — used by both Action and direct-CLI operators.
- **Content outline (directions; the authoritative operation catalog lives in `reconciliation`, this guide directs the granting):**
  - Prerequisites: ability to author IAM policies/roles; knowledge of the KMS key(s) SOPS uses and any customer-managed key protecting the target secrets.
  - Fact-boundary note (one line, links out): the tool accepts **no** credential inputs — credentials resolve only from the standard AWS SDK chain (env, shared config/profile, SSO, IRSA/IMDS, container/instance roles); only `--aws-region`/`--aws-profile` steer it and the Action forwards only `--aws-region`. Why → `security-and-trust`.
  - Step 1 — grant the Secrets Manager actions the tool calls, scoped by resource ARN to your prefix where IAM permits: `ListSecrets` (discovery, account-level), and `DescribeSecret`, `GetSecretValue`, `CreateSecret`, `PutSecretValue`, `RestoreSecret`, `DeleteSecret` on `arn:aws:secretsmanager:REGION:ACCT:secret:<prefix>/*`. See `reconciliation` for the full operation list and why each is issued.
  - Step 2 — **include `secretsmanager:TagResource`.** *(Resolves the DESIGN-vs-adapter discrepancy decisively.)* The tool makes no standalone `TagResource` API call — the three ownership tags ride the `CreateSecret` request — but AWS requires the `secretsmanager:TagResource` permission to create a secret *with* tags. Grant it. Note the operational consequence (owned by `ownership-and-scope`): tags are written at create time only; there is no post-create tag-repair path.
  - Step 3 — grant KMS limited to the keys SOPS needs to decrypt your documents (per each file's own metadata) and any customer-managed key on the Secrets Manager secrets (`kms:Decrypt`, and `kms:GenerateDataKey` where the SDK writes). The tool needs no IAM, OIDC-provider, or KMS administration permissions.
  - Step 4 — ARN-scoping guidance: wildcard the prefix path and account for the random 6-character suffix AWS appends to secret ARNs.
  - Step 5 — pick a credential delivery path: GitHub Actions assumes a role via AWS OIDC (hand off to `deploy-with-github-actions`); a bastion or local admin uses a shared profile/SSO. Confirm a region resolves — `Load` fails fast on an empty region or an unusable credential chain.
  - Verification: run `sops-aws-sync plan` (read-only) against a scratch prefix; a converged/`observation`-clean result confirms access.
  - Troubleshooting: `AWS configuration is invalid` → the region did not resolve; `AWS credential loading failed` → no usable credentials; an `AccessDenied` on one operation → add that specific action; secrets created outside the prefix are invisible by scope, not a permissions problem.
- **Excludes:** the descriptive operation catalog and *why each is called* → `reconciliation`; generic "create an IAM role / configure an OIDC provider" → upstream AWS docs (low-hanging fruit); workflow-side permissions and OIDC wiring → `deploy-with-github-actions`; the auth-boundary rationale → `security-and-trust`; flag defaults for region/profile → `configuration`.
- **Cross-links:** `deploy-with-github-actions`; `reconciliation`; `security-and-trust`; `configuration`; `diagnose-and-recover`.

---

### 3.5 `docs/how-to/verify-a-release-binary.md` — How to verify a downloaded release binary

- **Type:** How-to.
- **Purpose:** Manually confirm a downloaded CLI binary's integrity and provenance before running it — for operators who install the binary directly rather than via the Action.
- **Reader moment:** Downloaded a raw release binary for a bastion, non-GitHub CI, air-gapped mirror, or audit, and must confirm it is genuine before executing it with AWS access.
- **Content outline:**
  - Prerequisites: a `gh` new enough to advertise the attestation policy flags; the release tag `vX.Y.Z`; the downloaded binary plus `checksums.txt`; authentication (public releases verify anonymously via `gh auth`; private repos need a token with `contents: read` and `attestations: read`). Installing `gh` is out of scope — link upstream.
  - Step 1 — identify the platform asset name `sops-aws-sync_<version>_<os>_<arch>`, `os` in `{linux, darwin}`, `arch` in `{amd64, arm64}`; only Linux and macOS are published.
  - Step 2 — recompute the binary's SHA-256 and confirm the matching `<64hex>  <assetName>` line in `checksums.txt`. This indirection is required because the attestation subject is `checksums.txt` — a hash-of-hashes — not each binary.
  - Step 3 — run `gh attestation verify` against `checksums.txt` with the project's pinned policy: `--repo meigma/sops-aws-sync`, `--signer-workflow meigma/sops-aws-sync/.github/workflows/attest.yml`, `--source-ref refs/tags/vX.Y.Z`, `--cert-oidc-issuer https://token.actions.githubusercontent.com`, `--deny-self-hosted-runners`, `--predicate-type https://slsa.dev/provenance/v1`, `--digest-alg sha256`; confirm the returned statement's subject digest binds your `checksums.txt`.
  - Step 4 — only after both checks pass, `chmod +x` and run `sops-aws-sync version` to confirm the embedded `version (commit) built date` matches the release.
  - Escape route: any mismatch — checksum, signer identity, source-ref, or a self-hosted signer — means **do not run the binary**; fetch a clean copy. A moved/deleted tag or a fork's release fails this by design.
  - Note: the Action performs exactly this checksum-plus-attestation verification automatically on every install and on every tool-cache hit, so this manual procedure is only for direct downloads.
- **Excludes:** *why* provenance is mandatory and what an attestation does and does not prove → `security-and-trust`; installing `gh` / general SLSA background → upstream (low-hanging fruit); release-pipeline internals that mint the attestation → contributor scope; the Action's automatic verification described as facts → `configuration` and `security-and-trust`; supported-platform enumeration as a lookup → `configuration`.
- **Cross-links:** `security-and-trust`; `deploy-with-github-actions`; `configuration`.

---

### 3.6 `docs/how-to/decommission-a-scope.md` — How to decommission or empty a scope safely

- **Type:** How-to.
- **Purpose:** Retire individual secrets or an entire scope through the tool's soft-delete path without triggering accidental mass deletion, and recover within the window if needed.
- **Reader moment:** An operator intentionally retiring some or all secrets in a scope, facing the empty-state gate and recovery-window behavior — the highest-risk operation.
- **Content outline:**
  - Prerequisites: write access to commit to the source repo; a working `sync` path (CLI or Action).
  - Retire a single secret: delete or `git rm` the committed `.sops.json`, commit, `sync`; the owned secret is scheduled for deletion **last** (after all creates/updates/restores) with the configured recovery window. Verify via `schedule-delete` count and `status: converged`. Deletions from a non-empty snapshot are **not** gated.
  - Empty an entire scope: a commit leaving zero desired files whose plan would schedule deletions is **blocked** with `status: invalid` and zero deletions unless you pass `--allow-empty` (Action `allow-empty: true`). This authorization is **one-shot** for the transition — once secrets are already scheduled, later runs converge without it.
  - Choose the recovery window: `--recovery-window-days` / `recovery-window-days` is an integer 7–30 (default 30), passed verbatim to AWS `ScheduleDeletion`; permanent/force deletion is unavailable by design.
  - Recover a mistaken deletion within the window: re-add the file at the same path (same derived name) and `sync` — the tool `Restore`s and then updates to the committed value in one run; past the window the secret is gone and a fresh create occurs instead.
  - Handle an ambiguous delete: an ambiguous `ScheduleDeletion` stops the run as `apply-failed` with no same-run retry — re-run `sync` (link `consistency-and-recovery` for why delete/restore fail safe rather than auto-retry).
  - Cautions: renaming a file is delete-old + create-new, so expect a `schedule-delete` plus a `create`, not a rename (link `reconciliation-model`); changing repository-id / source-root / secret-prefix **abandons** (does not delete) prior secrets under the old scope (link `ownership-and-scope`).
  - Escape route: if a deletion was unintended, act before the window elapses and verify in AWS that the secret shows "scheduled for deletion," not already gone.
- **Excludes:** *why* deletion is soft and restore is a two-cycle operation → `reconciliation-model`; *why* the empty-state gate exists → `reconciliation-model`; the `--allow-empty`/`recovery-window-days` definitions and syntax → `configuration`; the `invalid` status and empty-gate rule as a spec → `reconciliation`/`results`; ambiguous-write theory → `consistency-and-recovery`; general failure recovery → `diagnose-and-recover`.
- **Cross-links:** `reconciliation-model`; `ownership-and-scope`; `consistency-and-recovery`; `configuration`; `reconciliation`; `results`; `diagnose-and-recover`.

---

### 3.7 `docs/how-to/diagnose-and-recover.md` — How to diagnose and recover from a failed run

- **Type:** How-to.
- **Purpose:** Turn a non-zero exit or non-converged status into a concrete remediation, keyed by symptom — since the tool deliberately withholds raw error detail.
- **Reader moment:** An operator whose `plan` or `sync` just failed (exit 3/4/5/6/130 or an Action failure message) and who needs to act now.
- **Content outline (symptom → remedy; links to `results` for what each code *is*, supplies what to *do*):**
  - Orient: read the exit code plus the report `status` and `verification` value (and any Action message); re-running is the general recovery path because completed AWS calls are never rolled back and a later run resumes from observed state (link `consistency-and-recovery`). Rely on JSON stdout logs and the report — stderr is redacted to a fixed warning.
  - **Exit 3 / invalid:** a bad config or desired state, an unknown config key (`UnmarshalExact` fails closed on typos), a nonexistent source-root, a non-object / duplicate-key / non-UTF-8 / over-64 KiB plaintext, a SOPS decryption failure, an empty snapshot with pending deletions and no `--allow-empty`, or a **`--report-file` write failure that masks the true outcome** — ensure the report directory is writable, fix the config/desired state, re-run.
  - **Exit 4 / conflict:** one secret blocks the whole run. Identify the class (`--show-resource-names` in a trusted log sink reveals which name; class catalog → `reconciliation`) and remediate: foreign / missing-tag / source-mismatch → rename the source or choose a different `secret-prefix` (the tool cannot retag or adopt it, since tagging is write-once); service-owned / rotation / replication / ambiguous-staging → resolve the condition in AWS (disable rotation, remove replicas) before syncing.
  - **Exit 5 / apply-failed or observation-failed:** a transient AWS or observation failure, or an ambiguous delete/restore — re-run `sync` (resumable-forward, no rollback happened) using the safe AWS error code + request-id from the logs for an AWS support case; create/update ambiguity self-resolves by re-observation, but an ambiguous restore or schedule-deletion requires your explicit re-run.
  - **Exit 6 / verification:** `inconclusive` means writes likely landed but AWS did not stabilize — raise `--verification-timeout` and/or re-run; do **not** re-apply from scratch. `failed` means a real mismatch or the self-healing rebuild budget was exhausted — confirm you have a single writer, then re-run.
  - **Exit 130 / interrupted:** SIGINT/SIGTERM or run-timeout, possibly during setup or SOPS decrypt — re-run; if it recurs at setup, raise `--run-timeout` or check SOPS/KMS reachability.
  - **Action messages:** `ownership or observed-state conflict` → 4; `apply did not complete safely` → 5; `verification did not converge` → 6; `was interrupted` → 130; `CLI exit status and redacted report disagree` → CLI/Action version skew (align the pinned ref with `cli-version`, link `security-and-trust`); the fixed `wrote a diagnostic to stderr` warning means stderr is suppressed by design.
  - Confirm recovery: after any fix, run `plan`; converged (zero operations) confirms the scope is healthy. Reassurance: a converged scope is a pure no-op and create/update are idempotent, so re-running is safe.
- **Excludes:** the authoritative meaning of each exit code, status, verification value, and conflict class → `results` and `reconciliation` (this guide links, it does not restate the catalog); *why* errors are opaque and redaction is enforced → `security-and-trust`; *why* ambiguous writes and inconclusive verification occur → `consistency-and-recovery`; intentional teardown (not a failure) → `decommission-a-scope`; IAM authoring for `AccessDenied` → `grant-aws-access`.
- **Cross-links:** `results`; `reconciliation`; `consistency-and-recovery`; `security-and-trust`; `configuration`; `decommission-a-scope`.

---

### 3.8 `docs/explanation/reconciliation-model.md` — About the reconciliation model

- **Type:** Explanation.
- **Purpose:** Give operators the core mental model of how the tool decides what to change in AWS and what it treats as "done," so plans and syncs are predictable and trustworthy.
- **Reader moment:** Away from the keyboard, building a trustworthy mental model before relying on the tool with production secrets, or making sense of a surprising plan.
- **Content outline (discursive; narrates the decision *as a concept*, defers every table to `reconciliation`):**
  - Source of truth: the set of committed `*.sops.json` blobs at one exact 40-character commit is the *only* desired state; the working tree, index, and unpushed changes are invisible; AWS is observed, never authoritative. The practical consequence: you must commit and push to make anything happen. Any ref you pass is pinned once to a full SHA and recorded as provenance.
  - Why plans are deterministic and reviewable: the reconciliation engine is pure business logic that imports no AWS/Git/SOPS APIs, so identical desired+observed inputs always yield the same sorted plan.
  - The plan as the *union* of desired names and tag-discovered scope members, and why the tool never enumerates the unbounded absent/absent namespace. A conceptual walk of the outcomes — present-in-Git → create/update/unchanged/restore; absent-from-Git-but-in-scope → scheduled deletion; anything untrusted → conflict — *described as a concept, not the lookup table*.
  - Names are derived, not stored: the secret name comes from the source path (prefix + the path minus source-root minus `.sops.json`, case preserved, no character substitution), so moving or renaming a file changes both the name and the source identity and the tool sees delete-old + create-new, never a rename (exact rules → `reconciliation`).
  - Why value comparison is byte-level canonical: desired canonical JSON is compared byte-for-byte with the live value, so formatting-only differences converge once and never churn, and a stored value that is not byte-identical canonical JSON is rewritten even when semantically equal.
  - Convergence defined: success is zero executable operations *and* zero conflicts; "unchanged" is summary metadata, not an operation; re-running a converged scope is a pure no-op — which is exactly why `plan` is a safe CI drift check.
  - Why deletion is soft and reversible: deletions are AWS scheduled deletions inside a recovery window (never force); removing a committed file schedules deletion; re-adding within the window restores rather than recreates; restore is a *separate cycle* because a scheduled secret's value cannot be read, so it is restore-then-one-update.
  - Deterministic safety ordering: restore → create → update → schedule-deletion, so a partial failure never removes access before creates/updates land.
  - The empty-state gate as a *concept*: an all-empty desired set that would schedule deletions is treated as dangerous and refused unless explicitly authorized — the reasoning behind the guardrail (the procedure lives in `decommission-a-scope`).
  - Closing: what a converged sync means and does not mean, pointing to `consistency-and-recovery` for drift, ambiguity, and non-convergence.
- **Excludes:** the exact desired×observed matrix, phase-order list, name charset/size limits, and the RFC 8785 caps → `reconciliation`; ownership tag mechanics, the scope digest, and the re-scoping trap → `ownership-and-scope`; eventual consistency, verification outcomes, ambiguous writes, no-rollback, interruption → `consistency-and-recovery`; confidentiality/redaction/provenance → `security-and-trust`; any procedure → the how-tos and tutorial.
- **Cross-links:** `ownership-and-scope`; `consistency-and-recovery`; `reconciliation`; `decommission-a-scope`; `tutorial/first-reconciliation`.

---

### 3.9 `docs/explanation/ownership-and-scope.md` — About ownership, scope, and fail-closed safety

- **Type:** Explanation.
- **Purpose:** Explain what makes a secret "managed by this tool," why the boundary is fail-closed, and why changing scope silently abandons secrets — the product's highest-stakes concept.
- **Reader moment:** Choosing repository-id / source-root / secret-prefix, or trying to understand why the tool refuses a secret or appears to have lost its secrets after a config change.
- **Content outline (discursive):**
  - The three reserved tags — `sops-aws-sync:managed-by` = `sops-aws-sync`, `sops-aws-sync:scope` = a digest, `sops-aws-sync:source` = a source-path hash — and the rule that a secret is owned only when managed-by **and** scope both match exactly; why ownership is tag-based and never implicitly adopted.
  - Scope identity as a versioned SHA-256 over the repository-id, cleaned source-root, and secret-prefix plus a `sops-aws-sync:scope:v1` label; why it is a digest and what "versioned" implies for a future label bump.
  - **The single most important operational trap:** changing repository-id, source-root, or secret-prefix mints a *new* scope that owns nothing prior, so old secrets become foreign and are abandoned while a fresh set is created under the new scope — severe, and not surfaced as an error. Treat scope inputs as an immutable identity; treat scope changes as migrations.
  - Source identity and collisions: the source tag is a hash of the cleaned repo-relative path, so two files that would map to the same AWS name collide as a foreign "source" conflict instead of overwriting each other.
  - Write-once tagging: reserved tags are set only at `CreateSecret` and there is no tag-repair path, so manually altered or removed reserved tags permanently orphan a secret from the tool's view; non-reserved tags are neither read for decisions nor disturbed.
  - The fail-closed philosophy: any single conflict blocks the *entire* plan before any mutation — the tool never adopts, overwrites, retags, restores, or deletes an unowned same-name secret, and it also refuses owned secrets that are service-owned, rotation-enabled/in-progress, replicated, or have ambiguous AWSCURRENT staging. Why refusal is safer than best-effort partial application, given the binary runs with the caller's AWS identity.
  - Implication: choose and freeze scope inputs early; a stable scope is what ties a repo checkout to its AWS-side secrets over time.
- **Excludes:** the enumerated conflict-class catalog with per-class triggers → `reconciliation`; step-by-step conflict remediation → `diagnose-and-recover`; the create/update/restore/delete decision logic and lifecycle → `reconciliation-model`; redaction of names in logs and the `--show-resource-names` tradeoff → `security-and-trust`; which IAM actions create/tag → `grant-aws-access`.
- **Cross-links:** `reconciliation-model`; `reconciliation`; `diagnose-and-recover`; `security-and-trust`.

---

### 3.10 `docs/explanation/consistency-and-recovery.md` — About consistency, verification, and recovery

- **Type:** Explanation.
- **Purpose:** Explain why a sync can be inconclusive, why re-running is the recovery path, and why correctness depends on a single writer per scope.
- **Reader moment:** Saw a non-converged result, or is designing how the tool runs (concurrency, retries, timeouts, interruption) and wants to operate confidently under failure.
- **Content outline (discursive; names concrete anchors sparingly for illustration and routes exact values to reference):**
  - AWS eventual consistency as the root cause: the list API can lag (AWS documents up to minutes), so the adapter merges the tag-filtered list with direct describe/get and cross-checks staging; a "successful sync" is explicitly not a globally strongly-consistent snapshot.
  - Why sync verifies independently: after apply it re-observes the whole scope on a fixed sub-second poll until the plan converges or a conflict appears, bounded by `verification-timeout`, re-observing each applied name even if the list omits it. The four verification outcomes *conceptually*: converged; inconclusive (writes likely landed but AWS did not stabilize → longer timeout or retry, not re-apply); failed (still wrong, or the rebuild budget was exhausted); not-run (aborted before mutation).
  - Idempotency and ambiguous writes, asymmetric by operation: a fresh per-write idempotency token is minted for create and update only, and on a lost-response mutation the tool re-observes real state and either accepts it, retries exactly once with the *same* token, or fails safe. Restore and schedule-deletion take no token, so their ambiguous outcomes fail safe as apply-failed and require a deliberate re-run. Why create/update auto-resolve but delete/restore do not.
  - Why external drift is repaired even at an unchanged commit: a later run mints a new token, so a stale token never blocks re-writing the committed value after drift.
  - Bounded self-healing: at most one benign plan rebuild is allowed; sustained concurrent churn terminates as verification-failed rather than looping forever.
  - The single-writer assumption: AWS offers no cross-secret transaction or compare-and-swap, so correctness depends on the caller enforcing exactly one active writer per scope and running only from trusted, serialized contexts.
  - No rollback, resumable-forward: completed AWS calls are never undone; on partial failure the tool stops before deletions, exits nonzero, and a later run resumes from observed state — sync is resumable-forward, not transactional.
  - Interruption semantics: SIGINT/SIGTERM and run-timeout both map to interrupted; during SOPS decrypt cancellation is terminal (the process exits without cooperatively cancelling the in-flight KMS call); no AWS mutation begins until every decrypt succeeds. Why setup-phase interruption and run-timeout are indistinguishable by exit code.
- **Excludes:** the exit-code, status, and verification value tables → `results`; symptom-to-action remediation → `diagnose-and-recover`; supply-chain provenance and secret redaction → `security-and-trust`; timeout flag names, defaults, and syntax → `configuration`; the happy-path decision model → `reconciliation-model`.
- **Cross-links:** `results`; `diagnose-and-recover`; `security-and-trust`; `reconciliation-model`; `deploy-with-github-actions`.

---

### 3.11 `docs/explanation/security-and-trust.md` — About the security and trust model

- **Type:** Explanation.
- **Purpose:** Explain the confidentiality boundary of the secrets and the supply-chain trust boundary of the binary — what an operator can and cannot trust, and why. (Unifies the two because the binary runs with the caller's AWS identity, so binary provenance and secret handling are the same question.)
- **Reader moment:** A security-conscious operator or reviewer deciding whether and how to trust the binary and what confidentiality to expect before granting it AWS access.
- **Content outline (discursive; corrects the dangerous "everything is secret" assumption):**
  - Framing: the CLI runs with the caller's AWS identity and handles decrypted secrets, so two boundaries are load-bearing — can I trust this binary, and what stays confidential.
  - Partial confidentiality — the misconception to correct: SOPS partial-encryption means JSON keys, repository paths, SOPS metadata, and explicitly unencrypted leaves are *plaintext in Git* and are governed by repository policy; only encrypted values are confidential. Do not assume an entire `.sops.json` is secret.
  - Integrity: the SOPS MAC is verified on decrypt against the committed blob bytes, so tampered ciphertext fails before any AWS write.
  - The memory boundary: decrypted bytes live only in process memory — never on disk, in a report, in argv, in env, or in logs — but Go and the AWS SDK give no reliable zeroization, so the guarantee is protection from egress channels, not scrubbing from memory.
  - Redaction is *structural*, not best-effort: safe error types and fixed-string decryption errors keep sensitive text (ARNs, SOPS diagnostics, provider identifiers) out of error values entirely; SOPS's own logging is suppressed; resource names appear as `resource-NNNN` ordinals unless `--show-resource-names` is set; the machine report is an exact-key allowlist; even `--log-level=debug` never leaks plaintext. This is why default logs and reports are safe to archive or paste.
  - The AWS auth boundary: neither the CLI nor the Action accepts credential material; AWS auth comes only from the ambient SDK chain the caller establishes first; the optional `github-token` authenticates release and attestation fetch only, is masked, and is never forwarded to AWS. Never grant AWS credentials to untrusted or `pull_request`-triggered runs.
  - What "attested" actually proves: the Action uses a binary only if its SHA-256 is in `checksums.txt` *and* `gh attestation verify` proves provenance against a pinned policy — the repo, the `attest.yml` signer workflow, the source ref/digest, the GitHub Actions OIDC issuer, SLSA provenance v1, and deny-self-hosted-runners. The attestation subject is `checksums.txt` (a hash-of-hashes), so a binary is bound through its checksum, and verification runs on both cache miss and cache hit, so a poisoned cache is caught. Fork or re-signed binaries cannot pass. What it proves (build identity + integrity) and what it does not (runtime behavior, the dependency supply chain).
  - Version pinning as a trust and correctness control: the version is fully pinned and immutable (exact semver, no "latest" or ranges); the CLI and Action move in lockstep; pin the Action to a release commit SHA, not a floating tag; only the latest release is supported (no backports).
  - What the model does not give you: it will not manage rotation/replication/service-owned secrets; it does not hide names or paths your Git repository exposes; and it is not AWS authentication.
- **Excludes:** the step-by-step manual verification procedure → `verify-a-release-binary`; workflow permission wiring and the OIDC step → `deploy-with-github-actions`; the IAM/KMS list → `grant-aws-access`; the report schema field list and the log-field vocabulary as lookups → `results` and `configuration`; release-pipeline internals → contributor scope; consistency/verification semantics → `consistency-and-recovery`.
- **Cross-links:** `verify-a-release-binary`; `deploy-with-github-actions`; `grant-aws-access`; `consistency-and-recovery`; `results`.

---

### 3.12 `docs/reference/configuration.md` — Configuration reference

- **Type:** Reference (neutral, factual voice throughout — no instructions, no rationale).
- **Purpose:** The authoritative, complete lookup of every CLI flag, environment variable, Action input/output, default, precedence rule, validation constraint, and stream contract.
- **Reader moment:** Writing or debugging a command line, config file, or workflow and needing exact names, defaults, precedence, and constraints.
- **Content outline (organized to mirror the product; the Action-forwards-to-CLI reality is the reason for a single unified surface, not two parallel docs):**
  - Subcommands: `plan` (read-only; supports plan-only `--detailed-exit-code`), `sync` (always mutates and always verifies — no skip-verify mode), `version` (loads no config or AWS); the bare root prints help. Every command sets `cobra.NoArgs` and rejects unexpected positional arguments.
  - A single unified option table, one row per knob, columns `[CLI flag | SOPS_AWS_SYNC_* env var | Action input | Default | Type & validation]`: `repository` (`.`), `revision` (`HEAD`, resolved to a full 40-hex SHA), `repository-id` (required, no default), `source-root` (`secrets`), `secret-prefix` (Action-required; trailing `/` trimmed), `max-encrypted-bytes` (`8MiB`, binary suffixes GiB/MiB/KiB/B only), `allow-empty` (false), `aws-region` (SDK chain), `aws-profile` (CLI-only), `recovery-window-days` (30; integer 7–30), `run-timeout` (`10m`, >0), `operation-timeout` (`30s`, >0), `verification-timeout` (`2m`, >0), `aws-max-attempts` (0, ≥0; CLI-only), `aws-max-backoff` (`0s`, ≥0; CLI-only), `log-level` (`info`; CLI-only), `log-format` (`json`; CLI-only), `show-resource-names` (false), `report-file` (CLI-only), `config` (flag-only, not env-bindable), `detailed-exit-code` (plan-only, no env).
  - The mechanical env-var rule: `SOPS_AWS_SYNC_` + flag uppercased with `-` and `.` → `_`; the sole exception is `--config` (no `SOPS_AWS_SYNC_CONFIG`).
  - Precedence: flag > `SOPS_AWS_SYNC_*` env > YAML config file > default; `UnmarshalExact` rejects any unknown key or type mismatch (fails closed).
  - Value-syntax specifics operators cannot guess: Go durations from explicit units (`5m`/`30s`/`1h30m`, not `300` or `5min`); recovery-window integer 7–30; `max-encrypted-bytes` binary suffixes only (decimal `MB` invalid); Action boolean inputs exactly lowercase `true`/`false`.
  - Action-only surface: `mode` (`plan`|`sync`), `fail-on-drift` (plan-only), `cli-version` (exact semver, no leading `v`, default tracked by release-please), `github-token` (default `${{ github.token }}`); the Action always appends `--log-format=json` and a temporary `--report-file`, and adds `--detailed-exit-code` only in plan mode with `fail-on-drift`.
  - Action outputs: `status`, `cli-version`, `git-revision`, `create-count`, `update-count`, `restore-count`, `scheduled-delete-count`, `unchanged-count`, `conflict-count`, `verification-status`, `redacted-report-path`.
  - Runner requirements: node24 runtime; Linux/macOS on X64/ARM64 only; required env `GITHUB_WORKSPACE`, `RUNNER_TEMP`, `RUNNER_OS`, `RUNNER_ARCH`, `GITHUB_SHA`, and `GITHUB_REPOSITORY` (the latter two are read unconditionally, even when `revision`/`repository-id` are supplied explicitly).
  - Cross-surface gaps stated explicitly: CLI flags with no Action input (`aws-profile`, `aws-max-attempts`, `aws-max-backoff`, `log-level`, `log-format`, `report-file`, `config`, `repository`, `max-encrypted-bytes`); Action inputs with no direct CLI flag (`mode`, `fail-on-drift`, `cli-version`, `github-token`).
  - The two-stream output contract: stdout carries structured logs and version/help text; stderr carries only Cobra usage and configuration-validation diagnostics (rejected values never echoed); machine results go only to `--report-file`; SOPS library logging is fully suppressed.
  - `--report-file` mechanics as an input fact: written atomically at mode 0600, written even on setup failure, and a write failure forces exit 3 (the *meaning* of that exit is in `results`).
  - `version` output format: `sops-aws-sync {version} ({commit}) built {date}`, injected at build time.
- **Excludes:** exit codes, statuses, verification values, and the report schema → `results`; source-format, naming, decision, and conflict-class facts → `reconciliation`; how to wire any value into a workflow → the how-tos; *why* precedence/redaction/pinning are designed this way → the explanations.
- **Cross-links:** `results`; `reconciliation`; `deploy-with-github-actions`; `grant-aws-access`.

---

### 3.13 `docs/reference/reconciliation.md` — Reconciliation reference

- **Type:** Reference (neutral, factual). This is the unified "rules" surface — the middle of the inputs → **rules** → outputs triad. It holds every fact needed to *predict a plan*, in one place, fixing the fact-scatter that split naming from the decision matrix in the base proposal.
- **Purpose:** State exactly how committed files become secret names, how desired × observed state becomes a plan, in what order operations run, which conditions block, and which AWS operations the tool issues.
- **Reader moment:** Authoring a secrets tree, or predicting precisely what a plan will do and why a given secret is refused.
- **Content outline:**
  - Source selection: only regular Git blobs whose names end in the literal suffix `.sops.json`, under source-root, discovered recursively; other encrypted extensions (`.enc.json`, `.sops.yaml`) are silently ignored (not errors); a matching path that is a symlink/submodule/non-regular mode is a hard error aborting the load; JSON-only; a `max-encrypted-bytes` ceiling applies before read.
  - Plaintext contract: SOPS MAC-verified; the decrypted document must be valid UTF-8, exactly one top-level JSON object (bare scalars/arrays rejected), with no duplicate member names at any depth and no trailing data; encoded to RFC 8785 (JCS); canonical value ≤ 65,536 bytes.
  - Name mapping: `secret name = secret-prefix + '/' + (repo-relative path − source-root − '.sops.json')`, case preserved, no character substitution; worked example — root `secrets`, prefix `/acme/payments`, file `secrets/production/database.sops.json` → `/acme/payments/production/database`; moving/renaming a file is delete + create; two files mapping to one name is a hard error.
  - Name constraints: non-empty, ≤ 512 bytes, characters restricted to `[A-Za-z0-9]` plus `/ _ + = . @ -`.
  - Ownership tags (fixed, not tunable): `sops-aws-sync:managed-by` = `sops-aws-sync`; `sops-aws-sync:scope` = SHA-256 over the `sops-aws-sync:scope:v1` label + repository-id + cleaned source-root + secret-prefix; `sops-aws-sync:source` = SHA-256 of the cleaned repo-relative source path. Owned requires managed-by **and** scope to match; tags are written only at `CreateSecret`; non-reserved tags are ignored and preserved.
  - Decision matrix (desired × observed → decision) as a table: Missing → Create; owned-active-string byte-equal → Unchanged, else Update; owned-active-binary / unknown / missing-AWSCURRENT → Update; owned-scheduled → Restore (plus one follow-up Update); absent-in-scope owned-active → ScheduleDeletion; absent owned-scheduled → Unchanged; Foreign / source-mismatch / service-owned / rotation / replication / staging / malformed → Conflict.
  - Operation phase order: Restore → Create → Update → ScheduleDeletion; Unchanged is metadata; ties broken by ascending name.
  - Empty-desired-state gate: an empty snapshot with any pending ScheduleDeletion and `allow-empty=false` yields `status: invalid` and zero mutations; the authorization is one-shot.
  - Conflict-class catalog with each triggering condition (descriptive): `ownership` (missing/wrong managed-by or scope), `source` (source tag differs, or two docs map to one name), `service-owned` (non-empty OwningService/Type), `rotation` (AWSPENDING or rotation enabled), `replication` (non-empty ReplicationStatus), `staging` (not exactly one AWSCURRENT / ambiguous staging), `payload`, `source-tag` (malformed). Any single conflict blocks the entire plan before mutation. *(Exit code 4 → this catalog is cross-linked from `results`.)*
  - Deletion behavior: scheduled deletion with `recovery-window-days` passed verbatim; `ForceDeleteWithoutRecovery` is never used.
  - AWS Secrets Manager operations the tool issues (the basis for the IAM policy in `grant-aws-access`): `ListSecrets` (tag-filtered on managed-by, `IncludePlannedDeletion=true`), `DescribeSecret`, `GetSecretValue` (AWSCURRENT), `CreateSecret` (with the three reserved tags + idempotency token), `PutSecretValue` (update), `RestoreSecret`, `DeleteSecret` (recovery window). No standalone `TagResource` call is made (tags ride `CreateSecret`).
  - SOPS specifics for this tool: decryption is delegated to getsops with format fixed to JSON and the document MAC verified; key backends (age/KMS/PGP/Vault) are whatever SOPS resolves from its own environment and the file's metadata — this tool adds no key configuration and layers a stricter content contract on top.
- **Excludes:** *why* the rules exist and the re-scoping trap → `reconciliation-model` and `ownership-and-scope`; how to resolve a specific conflict → `diagnose-and-recover`; how to turn the operations list into an IAM policy → `grant-aws-access`; flag defaults such as the recovery-window range → `configuration`; exit codes, report schema, run statuses → `results`.
- **Cross-links:** `reconciliation-model`; `ownership-and-scope`; `grant-aws-access`; `diagnose-and-recover`; `configuration`; `results`.

---

### 3.14 `docs/reference/results.md` — Results and exit codes reference

- **Type:** Reference (neutral, factual). The "outputs" end of the triad, and the *one place* an operator scripting CI decodes a run — with the exit ↔ status ↔ verification ↔ Action-message contract co-located as a single section (grafting operator-task-first's best consolidation as a section, not a mega-doc).
- **Purpose:** The authoritative catalog of every exit code, run status, verification value, report field, and Action failure message the tool can emit.
- **Reader moment:** Scripting CI on exit codes, parsing the report file, or decoding a status while diagnosing a run.
- **Content outline:**
  - Exit-code table: `0` success/converged; `2` plan drift (only with plan `--detailed-exit-code` or Action `fail-on-drift`); `3` invalid config/desired state **or** a forced override when report-file writing fails; `4` ownership/observed conflict; `5` apply-failed or observation-failed (also the fallback for unrecognized outcomes); `6` verification-failed or verification-inconclusive; `130` interrupted (including run-timeout during setup, indistinguishable from SIGINT/SIGTERM).
  - Status catalog, one line each: `converged`, `drift` (plan only), `conflict`, `invalid`, `apply-failed`, `observation-failed`, `interrupted`, `verification-failed`, `verification-inconclusive`.
  - Verification catalog, one line each: `not-run`, `converged`, `failed`, `inconclusive`.
  - Report schema `sops-aws-sync/report/v1`, field-by-field: `schema_version` (must equal the literal), `tool_version`, `git_revision` (may be empty for very early failures), `status`, `counts{create,update,restore,schedule_delete,unchanged,conflicts}` (six non-negative integers), `verification`, `duration_ms`; plus the guarantees — written atomically at mode 0600, written even on setup failure, size-bounded ≤ 64 KiB, exact-key contract that rejects extra fields.
  - The exit-code ↔ report-status agreement contract: the two must agree; a disagreement (a version-skew signal) surfaces as `CLI exit status and redacted report disagree` and publishes zero outputs.
  - Action failure-message map (co-located here): exit 3 → "configuration or desired state"; 4 → "ownership or observed-state conflict"; 5 → "apply did not complete safely"; 6 → "verification did not converge"; 130 → "was interrupted"; the fixed `wrote a diagnostic to stderr` warning; unexpected internal errors → `sops-aws-sync Action execution failed`.
  - Log-field vocabulary and streams recap (as facts, cross-referenced to `configuration`): default logs carry counts, durations, operation kinds, retry decisions, and safe AWS metadata (error code, fault class, request-id); resource names appear as `resource-NNNN` ordinals by default.
  - Cross-reference note: the *conflict-class* catalog (the causes behind exit 4) lives in `reconciliation`; this document states what an exit code / status / verification value *is*, not the taxonomy of conflicts and not what to do about them.
- **Excludes:** per-symptom remediation → `diagnose-and-recover`; *why* verification, idempotency, and ordering behave this way → `consistency-and-recovery` and `reconciliation-model`; the conflict-class triggers → `reconciliation`; flag/input definitions → `configuration`; *why* the report is redacted → `security-and-trust`.
- **Cross-links:** `configuration`; `reconciliation`; `diagnose-and-recover`; `consistency-and-recovery`; `security-and-trust`.

---

## 4. Type-boundary rules

These are the concrete content-routing laws that keep the set pure. Each fact/concept/procedure has exactly one home.

- **Exit codes, run statuses, verification values, report schema, and the Action failure-message vocabulary** → only `reference/results.md`. Explanations narrate them conceptually ("the four verification outcomes"); how-tos link to them; nothing else catalogs them.
- **Every flag, env var, Action input/output, precedence rule, and the stdout/stderr/report stream contract** → only `reference/configuration.md`.
- **Source selection, name mapping, name/value constraints, ownership tag keys and derivation, the decision matrix, phase order, the conflict-class catalog, and the AWS operations list** → only `reference/reconciliation.md`. This is the single "predict a plan" surface.
- **The AllowEmpty gate has a deliberately three-way split, and it is the model for every fact/concept/procedure trio:** the *rule* (empty snapshot + pending deletions + `allow-empty=false` → `invalid`, one-shot) is a fact in `reference/reconciliation.md`; the *reasoning* (why empty is treated as dangerous) is in `explanation/reconciliation-model.md`; the *procedure* to intentionally empty a scope is in `how-to/decommission-a-scope.md`. No document holds more than its slice.
- **Conflict decoding is split by nature, not duplicated:** exit `4 → "a conflict blocked the plan"` is a run-level fact in `results.md`; *which* conflict classes exist and what triggers each is a planning-rules fact in `reconciliation.md`; *what to do* about a given class is a procedure in `diagnose-and-recover.md`; *why* the boundary is fail-closed is a concept in `ownership-and-scope.md`. `results.md` cross-links the catalog rather than reprinting it, and `diagnose-and-recover.md` is the operator's true entry point for a failed run, which mitigates the two-reference lookup.
- **IAM/KMS** splits three ways: the *operation list* (fact) is in `reconciliation.md`; the *how to grant it* (task, including the decisive `TagResource` resolution and prefix-ARN scoping) is in `grant-aws-access.md`; the *why auth is the caller's responsibility* (concept) is in `security-and-trust.md`. The how-to directs granting and links to the reference for the authoritative list — it does not re-catalog operations.
- **Redaction** splits three ways: *what fields exist* and the `resource-NNNN` ordinal are facts in `configuration.md` and `results.md`; the `--show-resource-names` flag definition is a fact in `configuration.md`; *why redaction is structural* (safe error types, suppressed SOPS logging) is a concept in `security-and-trust.md`.
- **Supply-chain verification** splits: the *procedure* (checksum + attestation policy flags) is in `verify-a-release-binary.md`; *what the attestation proves and why forks fail* is a concept in `security-and-trust.md`; the *runtime/platform/asset facts* are in `configuration.md`.
- **Explanations may name a concrete value once, as illustration, but must never catalog.** An explanation may say "a fixed sub-second poll" or "the GitHub Actions OIDC issuer" to ground a point; it must not list the exit codes, the verification values, or the full attestation flag set — those are reference. (This directly fixes the base proposal's slip of dropping enumerable reference values into explanation prose.)
- **The tutorial teaches one path with no options or alternatives.** Every "why" is a link out. It never introduces a flag as a choice (which is why the delete/restore demo keeps the scope non-empty and never touches `--allow-empty`).
- **References are descriptive only** — no instructions, no rationale, no "verify against your account" hedges. Where a fact was genuinely uncertain (the `TagResource` question), it is *resolved* in the how-to that owns the task, not left as an advisory note in a reference.
- **Every document carries a *deliberate exclusions* list** naming banned adjacent content and its owning document. This is the mechanism that keeps boundaries enforced as the docs are groomed.

---

## 5. Deliberately omitted topics

Considered and rejected, with reasons.

- **"What is SOPS", "install SOPS", "set up age/KMS/PGP/Vault keys."** Generic, upstream-owned, answerable by a one-line prompt. The tool's narrower contract (JSON-only, `.sops.json` suffix, single top-level object, no duplicate keys, 64 KiB canonical cap, MAC-verified) is stated in `reconciliation` and shown once in the tutorial; key setup is a linked prerequisite.
- **"Install the `gh` CLI" and generic AWS credential/OIDC provider setup.** Low-hanging fruit with authoritative vendor docs. Only the tool-specific slices survive: `gh` must advertise the attestation policy flags (`verify-a-release-binary`), and AWS auth comes solely from the SDK chain (`grant-aws-access`, `security-and-trust`).
- **A standalone `aws-permissions` reference.** Merged: the operation list is a fact in `reconciliation`; the *how to grant it* is `grant-aws-access`. A fourth reference would duplicate the operations list and the region/profile facts and inflate the reference count against the "smaller set" brief.
- **Standalone CLI-reference and Action-reference documents (splitting the two surfaces).** Rejected as a false economy that both fragments the "one tool, two surfaces" model and duplicates the shared knobs (durations, recovery-window-days, allow-empty, show-resource-names, secret-prefix, source-root). One unified `configuration` reference with a row-per-knob table mirrors the product's defining fact — the Action forwards to the CLI.
- **Standalone references for exit codes, the report schema, conflict classes, operation ordering, and naming (five thin docs).** Each is too thin to stand alone. Merged into the triad: exit codes + report schema + status/verification + Action messages → `results`; naming + decision matrix + conflict classes + phase order + AWS ops → `reconciliation`.
- **Standalone explanations for canonical RFC 8785 JSON, idempotency/fingerprints, logging/redaction, configuration precedence, and CLI/Action version lockstep (five facets).** Each is a facet of a larger coherent idea, not its own topic. Byte-equality drift folds into `reconciliation-model`; idempotency and ambiguous writes into `consistency-and-recovery`; redaction and version pinning into `security-and-trust`; precedence is a dry fact in `configuration`. Splitting them would create shallow, overlapping reads.
- **A standalone runner-requirements reference and a "V1 non-goals" reference.** Short fact clusters, not documents: runner requirements fold into `configuration`; unsupported secret states (rotation, replication, service-owned, staging) are already the conflict-class catalog in `reconciliation`; platform/format limits fold into their natural reference sections; the landing surfaces the non-goals as pointers.
- **Separate "gate CI on drift" and "override github-token" how-tos.** Both are variations of one goal — running the Action in a workflow — folded into `deploy-with-github-actions` as concise variations. A separate goal doc would overlap heavily.
- **Separate "resolve a conflict" and "recover from an interrupted/inconclusive run" how-tos.** Same type, same reader moment ("my run did not succeed, what now"). Merged into the symptom-indexed `diagnose-and-recover`.
- **A second tutorial (e.g. a guided first CI deployment).** Deploying to CI is a real-world, varied task with genuine failure and trust decisions — a how-to, not a controllable, consequence-free lesson. One tutorial (the local reconcile loop) is sufficient to build the mental model experientially.
- **A "basic install and run once" tutorial.** Low value given the README quickstart and the deploy how-to; replaced by the higher-value lifecycle tutorial that demonstrates convergence, drift repair, and soft-delete/restore.
- **A "run the CLI in non-GitHub CI" how-to.** The tool is primarily consumed via the Action; a generic-CI guide would mostly duplicate `configuration` (flags, precedence) and `results` (exit-code branching). Direct-CLI operators are served by the tutorial, those two references, and `verify-a-release-binary`.
- **Contributor and release-pipeline documentation** (release-please, GoReleaser, `attest.yml`, `ghd.toml`, draft-release lifecycle, four-file version lockstep, moon/mise toolchain, commit conventions). Out of the operator audience and explicitly excluded by the brief; `CONTRIBUTING.md` already covers it. Only the operator-consumable trust guarantees these produce (pinned versions, fail-closed provenance, CLI/Action lockstep) surface in `security-and-trust`.

---

## 6. README's role

The top-level `README.md` remains the **GitHub front door**, not a docs page, and is intentionally not duplicated inside `docs/`:

- A one-paragraph what/why, the safety-model highlights, and a single minimal Action snippet (the copy-paste starting point).
- Build-from-source and the quick-start example — the fast on-ramp for someone evaluating the tool on the repo page.
- A routing hand-off into `docs/`: new operators → the tutorial; deployers → the how-tos; the model → the explanation spine; lookups → the references.

`docs/index.md` is the **docs-site map** (the internal router), distinct from the README's front-porch pitch — the two do not overlap, so the docs never restate the README's quickstart. `SECURITY.md` (private vulnerability reporting) and `CONTRIBUTING.md` stay out of scope as contributor/community-health files; site tooling (mkdocs, etc.) is out of scope by instruction. The only content lifted *from* the README into the operator set is its "Safety model" section, whose concepts are the proper, expanded subject of the four explanations.


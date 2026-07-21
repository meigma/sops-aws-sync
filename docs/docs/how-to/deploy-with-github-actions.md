# How to deploy sops-aws-sync with GitHub Actions

Wire the Action into a workflow that plans on pull requests and syncs on
protected merges, with AWS OIDC authentication, single-writer concurrency, and
a pinned, provenance-verified CLI.

## Prerequisites

- An AWS role assumable from GitHub via OIDC, carrying the least-privilege
  Secrets Manager and KMS policy from
  [How to grant sops-aws-sync least-privilege AWS access](./grant-aws-access.md). Use a
  read-only role for the plan job and a write-capable role for the sync job.
- The GitHub-to-AWS OIDC trust relationship already established. Setting up the
  OIDC provider and role trust policy is generic AWS/GitHub work — follow
  [GitHub's OpenID Connect guide for Amazon Web Services][gh-oidc] and do not
  re-derive it here.
- SOPS decryption keys reachable from the runner (age/KMS/PGP/Vault, resolved
  by SOPS from its own environment and each file's metadata).
- Committed `*.sops.json` files laid out under a source root, per the
  [Reconciliation reference](../reference/reconciliation.md).
- A chosen, frozen ownership scope — the `repository-id`, `source-root`, and
  `secret-prefix` triple. Treat these as an immutable identity; changing any of
  them silently abandons the secrets under the old scope. See
  [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).
- The full release commit SHA to pin the Action to (the commit behind the exact
  release tag you intend to run).

The Action verifies the CLI binary's SHA-256 checksum and SLSA provenance
automatically on every install and every tool-cache hit, so you never perform
manual verification in CI. For direct (non-Action) CLI use, see
[How to verify a downloaded release binary](./verify-a-release-binary.md); for
what that verification proves, see
[About the security and trust model](../explanation/security-and-trust.md).

## Wire the shared building blocks

The plan and sync workflows share the same five decisions. Apply all of them in
both.

### 1. Lock down workflow permissions

Set no permissions at the workflow top level and grant the minimum per job:

```yaml
permissions: {}

jobs:
  reconcile:
    permissions:
      contents: read      # read the committed .sops.json blobs
      id-token: write     # mint the GitHub OIDC token for AWS
      attestations: read  # read a private repo's CLI attestations
```

`contents: read` is always required. `id-token: write` is required only when
AWS authentication uses OIDC (the recommended path). `attestations: read` is
required only when provenance verification must read a private repository's
attestations; the default `${{ github.token }}` verifies a public same-repo
install without it, and the scope is harmless when unused.

### 2. Authenticate to AWS first

Add the AWS credential step **before** the sops-aws-sync step. The Action
performs no AWS authentication of its own — it forwards only `--aws-region` and
relies on the ambient AWS SDK credential chain the runner already holds. Missing
or insufficient credentials therefore surface later as a CLI runtime failure
(an ownership/observed-state conflict or an apply failure), not as an Action
input error. See [How to diagnose and recover from a failed run](./diagnose-and-recover.md).

```yaml
- uses: aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d # v6.2.2
  with:
    role-to-assume: ${{ secrets.SOPS_AWS_SYNC_ROLE_ARN }}
    aws-region: us-west-2
```

The Action never receives credential material. It accepts no access key, secret
key, or session token input — only `--aws-region` is forwarded.

### 3. Check out the exact revision

Check out with full history and without leaving Git credentials on disk, so the
committed blobs at the resolved revision are present:

```yaml
- uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
  with:
    fetch-depth: 0
    persist-credentials: false
```

The Action defaults `revision` to `GITHUB_SHA` and `repository-id` to
`GITHUB_REPOSITORY`. The tool reads the committed blob at the resolved revision,
never the working tree — the checkout must contain that commit.

### 4. Pin the Action to a release commit SHA

Reference the Action by the full commit SHA behind an exact release tag, with a
version comment. Do not use `@v1`, `@master`, or any floating ref:

```yaml
- uses: meigma/sops-aws-sync@<full-release-commit-sha> # v0.1.1
```

Leave `cli-version` unset. Each pinned release ref carries the exact CLI version
it was built and tested against as its default. Overriding `cli-version` to a
value that does not match the pinned ref decouples the CLI from the Action and
can fail installation or trip the `CLI exit status and redacted report disagree`
error. The CLI and the Action move in lockstep — see
[About the security and trust model](../explanation/security-and-trust.md) for
why pinning is a trust and correctness control.

### 5. Serialize writers per scope

The tool assumes exactly one active writer per ownership scope; AWS offers no
cross-secret transaction to fall back on. Enforce this with a `concurrency`
group keyed to the scope (not to the branch or PR), and never cancel a run in
flight:

```yaml
concurrency:
  group: sops-aws-sync-production
  cancel-in-progress: false
```

Run `sync` only from a protected default-branch merge or a protected-environment
deployment — never from `pull_request` code holding AWS credentials. For why the
single-writer assumption is load-bearing, see
[About consistency, verification, and recovery](../explanation/consistency-and-recovery.md).

## Variation A — plan on pull requests with drift gating

Run a read-only `plan` on pull requests and fail the check when executable drift
is found, gating the merge. `fail-on-drift` is valid only in plan mode and makes
drift return exit code 2. Boolean inputs must be exactly lowercase `true` or
`false`.

```yaml
# .github/workflows/sops-plan.yml
name: sops-aws-sync plan

on:
  pull_request:
    paths:
      - secrets/**

permissions: {}

concurrency:
  group: sops-aws-sync-plan-${{ github.ref }}
  cancel-in-progress: true

jobs:
  plan:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
      attestations: read
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
        with:
          fetch-depth: 0
          persist-credentials: false

      - uses: aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d # v6.2.2
        with:
          role-to-assume: ${{ secrets.SOPS_AWS_SYNC_PLAN_ROLE_ARN }}
          aws-region: us-west-2

      - id: plan
        uses: meigma/sops-aws-sync@<full-release-commit-sha> # v0.1.1
        with:
          mode: plan
          fail-on-drift: true
          source-root: secrets
          secret-prefix: /example/production
          aws-region: us-west-2
```

Give the plan job a read-only AWS role and restrict it to same-repository pull
requests — GitHub does not issue an OIDC token to forked-PR workflows, and a
read-only role limits the blast radius of PR code. A converged plan passes; a
drift plan exits 2 and fails the check.

## Variation B — sync on protected merges

Run `sync` on a push to the protected default branch. Do **not** set
`fail-on-drift` in sync mode — combining the two fails input parsing with
`Input fail-on-drift is valid only in plan mode`. Gate downstream steps on the
`status` output.

```yaml
# .github/workflows/sops-sync.yml
name: sops-aws-sync sync

on:
  push:
    branches: [main]
    paths:
      - secrets/**

permissions: {}

concurrency:
  group: sops-aws-sync-production
  cancel-in-progress: false

jobs:
  sync:
    runs-on: ubuntu-latest
    environment: production
    permissions:
      contents: read
      id-token: write
      attestations: read
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
        with:
          fetch-depth: 0
          persist-credentials: false

      - uses: aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d # v6.2.2
        with:
          role-to-assume: ${{ secrets.SOPS_AWS_SYNC_ROLE_ARN }}
          aws-region: us-west-2

      - id: sync
        uses: meigma/sops-aws-sync@<full-release-commit-sha> # v0.1.1
        with:
          mode: sync
          source-root: secrets
          secret-prefix: /example/production
          aws-region: us-west-2

      - name: Continue only on a converged sync
        if: steps.sync.outputs.status == 'converged'
        run: |
          echo "status=${{ steps.sync.outputs.status }}"
          echo "verification=${{ steps.sync.outputs.verification-status }}"
```

The `environment: production` binds the job to a protected environment; configure
its protection rules (required reviewers, deployment branch restrictions) to
approve privileged syncs. `cancel-in-progress: false` guarantees a running sync
is never cancelled mid-flight — an interrupted sync must be re-run; see
[How to diagnose and recover from a failed run](./diagnose-and-recover.md).

## Variation C — private or cross-repository installs

For a public same-repository install, the default `github-token` (`${{
github.token }}`) is sufficient and needs no override. For a private or
cross-repository consumer, supply a token that can read the sops-aws-sync
repository's contents and attestations:

```yaml
      - id: sync
        uses: meigma/sops-aws-sync@<full-release-commit-sha> # v0.1.1
        with:
          mode: sync
          secret-prefix: /example/production
          github-token: ${{ secrets.SOPS_AWS_SYNC_READ_TOKEN }}
```

This token authenticates release-metadata reads, asset download, and
`gh attestation verify` only. It is masked before any installation work, is
passed to `gh` through an isolated configuration directory, and is never
forwarded to AWS. Do not reuse an AWS-privileged token here.

## Variation D — self-hosted and container runners

The Action runs on the `node24` runtime and installs the CLI only for
Linux/macOS on X64/ARM64 runners. A self-hosted or container runner must
therefore provide:

- Node 24 support for the Action runtime.
- A supported platform: `Linux` or `macOS`, `X64` or `ARM64`. Other operating
  systems or architectures fail with an explicit "supports only Linux and
  macOS" / "only X64 and ARM64" message.
- A `gh` CLI present and new enough to advertise the attestation policy flags
  the Action probes; an old or missing `gh` fails with "A compatible GitHub CLI
  is required for attestation verification." Installing `gh` is out of scope —
  follow the upstream GitHub CLI documentation.
- The runner environment variables the Action reads: `GITHUB_WORKSPACE`,
  `RUNNER_TEMP`, `RUNNER_OS`, `RUNNER_ARCH`, `GITHUB_SHA`, and
  `GITHUB_REPOSITORY` are all always required. Supplying the `revision` and
  `repository-id` inputs does not remove the requirement for `GITHUB_SHA` and
  `GITHUB_REPOSITORY`. Container images that strip any of these fail before
  installation.

The full enumeration of inputs, outputs, defaults, and validation rules is in
the [Configuration reference](../reference/configuration.md).

## Consume the results

The step publishes a fixed set of outputs sourced only from the redacted machine
report: `status`, `cli-version`, `git-revision`, `create-count`,
`update-count`, `restore-count`, `scheduled-delete-count`, `unchanged-count`,
`conflict-count`, `verification-status`, and `redacted-report-path`. Read them
from `steps.<id>.outputs.<name>` to gate or annotate downstream work, as the
sync workflow does above.

The Action also writes a two-table job summary built only from the validated
report — the first table covering status, CLI version, git revision, and
verification, the second the six operation counts — safe to leave visible.

To retain the redacted report as a build artifact, upload it from
`redacted-report-path` within the same job (the file lives under `RUNNER_TEMP`
and is cleared when the job ends):

```yaml
      - name: Archive the redacted report
        if: always()
        uses: actions/upload-artifact@<full-commit-sha> # v4 — pin to a SHA
        with:
          name: sops-aws-sync-report
          path: ${{ steps.sync.outputs.redacted-report-path }}
```

The report and default logs carry no secret values, names, or paths, so they are
safe to archive. Because raw CLI stderr is suppressed to a single fixed warning
(`sops-aws-sync wrote a diagnostic to stderr`), debug from the JSON stdout log
lines and the redacted report — not from stderr. The output and report field
lists are catalogued in the [Configuration reference](../reference/configuration.md)
and [Results and exit codes reference](../reference/results.md).

## Verify the deployment

A clean sync reports `status: converged` and `verification-status: converged`,
and the step succeeds (exit 0). Confirm both outputs on a merge run:

```yaml
      - name: Assert convergence
        run: |
          test "${{ steps.sync.outputs.status }}" = converged
          test "${{ steps.sync.outputs.verification-status }}" = converged
```

A converged scope is idempotent (see
[About the reconciliation model](../explanation/reconciliation-model.md)) — a
green pipeline that reports `unchanged-count` equal to your secret count and
zero mutations confirms the scope is healthy. Any other status or a non-zero exit means the run did not
converge — go to
[How to diagnose and recover from a failed run](./diagnose-and-recover.md).

## Related

- [How to grant sops-aws-sync least-privilege AWS access](./grant-aws-access.md)
- [How to verify a downloaded release binary](./verify-a-release-binary.md)
- [How to diagnose and recover from a failed run](./diagnose-and-recover.md)
- [Configuration reference](../reference/configuration.md)
- [Results and exit codes reference](../reference/results.md)
- [About the reconciliation model](../explanation/reconciliation-model.md)
- [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md)
- [About consistency, verification, and recovery](../explanation/consistency-and-recovery.md)
- [About the security and trust model](../explanation/security-and-trust.md)

[gh-oidc]: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services

---
title: GitHub Action
---

# GitHub Action

The Action is a transport adapter around the released Go CLI. It does not read
Git, decrypt SOPS, assume an AWS role, call AWS directly, or implement
reconciliation policy.

## Trusted synchronization workflow

Use a protected default-branch merge or a manual dispatch guarded by a protected
environment. Never run privileged synchronization against untrusted pull
request code. Serialize runs with a concurrency group unique to the repository
identity, source root, Region, and secret prefix.

```yaml
permissions:
  contents: read
  id-token: write
  attestations: read

concurrency:
  group: sops-aws-sync-production
  cancel-in-progress: false

jobs:
  sync:
    runs-on: ubuntu-24.04
    environment: production
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
        with:
          fetch-depth: 0
          persist-credentials: false

      - uses: aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d # v6.2.2
        with:
          role-to-assume: ${{ secrets.SOPS_AWS_SYNC_ROLE_ARN }}
          aws-region: us-west-2

      - uses: meigma/sops-aws-sync@<full-release-commit-sha> # v0.1.1
        with:
          mode: sync
          source-root: secrets
          secret-prefix: /example/production
          aws-region: us-west-2
```

Pin the Action to the full commit behind the exact release tag. The Action's
metadata defaults `cli-version` to the paired release so the Git ref and binary
contract stay together.

## Authentication

The caller authenticates first. The Action has no AWS credential inputs and
does not assume roles. GitHub-hosted workflows should use OIDC through
`aws-actions/configure-aws-credentials`; add `id-token: write` only for that
authentication step. The Go SDK then consumes the resulting standard chain.

Set `contents: read`. Current `gh attestation verify` also requires an
authenticated token for public artifacts, so `github-token` defaults to
`${{ github.token }}`. For a private cross-repository producer, pass a separate
token with read access to `meigma/sops-aws-sync` contents and attestations. The
token is masked and is used only by release download and provenance
verification; it is never forwarded to the CLI.

## Installation boundary

The Action accepts only an exact semantic `cli-version`. Downloads and cache
hits are both checked against `checksums.txt` and GitHub provenance for the
expected repository, release tag, commit, signer workflow, GitHub OIDC issuer,
and GitHub-hosted signer. It fails instead of using `latest`, a PATH binary, a
source build, an arbitrary URL, or an unverifiable cache entry.

Supported runners are Linux and macOS on amd64 and arm64. Self-hosted runners
must provide a compatible authenticated GitHub CLI.

## Inputs and outputs

`mode` defaults to non-mutating `plan`; set `mode: sync` explicitly to mutate.
`fail-on-drift` is valid only in plan mode. The remaining inputs mirror the
stable CI subset of the CLI: source root, secret prefix, revision, repository
identity, Region, recovery window, timeouts, empty-state authorization,
resource-name disclosure, exact CLI version, and GitHub token.

Outputs contain only status, CLI version, Git revision, operation and conflict
counts, verification status, and the redacted report path. Resource names and
secret material are excluded unless the CLI's explicit identifier-disclosure
input is enabled.

# sops-aws-sync

`sops-aws-sync` reconciles SOPS-encrypted JSON or YAML documents from one exact
Git commit with an explicitly owned namespace in AWS Secrets Manager. The Go
CLI plans and applies create, update, restore, and recovery-window deletion
operations; the Node 24 GitHub Action installs and verifies the paired CLI
release before invoking it without a shell.

## Installation

Immutable releases contain Linux and macOS binaries for amd64 and arm64,
`checksums.txt`, per-binary SBOMs, and GitHub provenance attestations. See
[how to verify a release binary](https://meigma.github.io/sops-aws-sync/how-to/verify-a-release-binary/)
before executing a downloaded binary.

To build from source with the repository toolchain:

```sh
mise install
moon run root:build
./bin/sops-aws-sync version
```

## CLI usage

Desired state comes from committed `.sops.json`, `.sops.yaml`, and `.sops.yml`
blobs under `--source-root`. The working tree and Git index are never
reconciled.

```sh
sops-aws-sync plan \
  --repository . \
  --revision HEAD \
  --repository-id meigma/example \
  --source-root secrets \
  --secret-prefix /example/production \
  --aws-region us-west-2

sops-aws-sync sync \
  --repository . \
  --revision HEAD \
  --repository-id meigma/example \
  --source-root secrets \
  --secret-prefix /example/production \
  --aws-region us-west-2
```

A YAML source contains one top-level mapping with JSON-compatible values:

```yaml
database:
  host: db.internal
  port: 5432
```

Encrypt it with SOPS before committing it. JSON and YAML inputs are both stored
as canonical JSON, so an encoding-only change does not create value drift or
change source ownership.

Run `sops-aws-sync plan --help` or `sops-aws-sync sync --help` for the complete
typed interface. Flags override `SOPS_AWS_SYNC_*` environment variables, which
override an explicitly selected YAML configuration file, which overrides
defaults.

## GitHub Action

Authenticate to AWS before invoking the Action. Privileged synchronization
must run only from trusted code and must serialize writers for each ownership
scope.

```yaml
permissions:
  contents: read
  id-token: write
  attestations: read

concurrency:
  group: sops-aws-sync-production
  cancel-in-progress: false

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

Replace the placeholder with the full commit behind the exact release tag. The
optional `github-token` input defaults to `${{ github.token }}` because current
GitHub CLI attestation verification requires authentication. A private
cross-repository consumer must provide a token scoped to read this repository's
contents and attestations.

## Safety model

- Desired state is fully read, decrypted, validated, and canonicalized before
  AWS mutation begins.
- Same-name secrets outside the exact reserved ownership tags fail closed.
- Deletions use a 7–30 day recovery window and always run after restore, create,
  and update operations. Force deletion is unavailable.
- Default logs and reports omit secret values, paths, secret names, provider
  identifiers, credentials, tokens, and raw service errors.
- AWS authentication comes from the standard AWS SDK chain; credentials are not
  Action inputs or application configuration.
- A successful sync means the fully paginated observed scope plus every desired
  and affected name re-plans to zero operations and zero conflicts. It is not a
  globally strongly consistent AWS snapshot.

## Documentation

The [operator documentation](https://meigma.github.io/sops-aws-sync/) is
organized by need:

- New to the tool: the
  [first reconciliation tutorial](https://meigma.github.io/sops-aws-sync/tutorial/first-reconciliation/)
  walks the plan, sync, and verify loop against a disposable scope.
- Deploying: how-to guides cover
  [GitHub Actions deployment](https://meigma.github.io/sops-aws-sync/how-to/deploy-with-github-actions/),
  [least-privilege AWS access](https://meigma.github.io/sops-aws-sync/how-to/grant-aws-access/),
  [release verification](https://meigma.github.io/sops-aws-sync/how-to/verify-a-release-binary/),
  [decommissioning a scope](https://meigma.github.io/sops-aws-sync/how-to/decommission-a-scope/),
  and [diagnosing failed runs](https://meigma.github.io/sops-aws-sync/how-to/diagnose-and-recover/).
- Understanding the system: explanations of the
  [reconciliation model](https://meigma.github.io/sops-aws-sync/explanation/reconciliation-model/),
  [ownership and scope](https://meigma.github.io/sops-aws-sync/explanation/ownership-and-scope/),
  [consistency and recovery](https://meigma.github.io/sops-aws-sync/explanation/consistency-and-recovery/),
  and the [security and trust model](https://meigma.github.io/sops-aws-sync/explanation/security-and-trust/).
- Looking something up: references for
  [configuration](https://meigma.github.io/sops-aws-sync/reference/configuration/),
  [reconciliation rules](https://meigma.github.io/sops-aws-sync/reference/reconciliation/),
  and [results and exit codes](https://meigma.github.io/sops-aws-sync/reference/results/).

## Development

The repository uses mise for its locked toolchain and Moon as the task front
door.

```sh
mise install
moon run root:check
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution workflow and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

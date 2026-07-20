# sops-aws-sync

`sops-aws-sync` reconciles SOPS-encrypted JSON documents from an exact Git commit with an explicitly owned namespace in AWS Secrets Manager.

Phase 2 provides the first durable Go CLI vertical slice: exact-commit reads, in-process SOPS decryption, strict JSON canonicalization, direct desired-name observation, create, update, no-op, and direct post-apply verification. Scope-wide discovery, restore, and scheduled deletion are intentionally deferred to the next delivery phase.

## Development

The repository uses mise for pinned tools and Moon as the task front door.

```sh
mise install
moon run root:check
```

Build or inspect the CLI directly:

```sh
moon run root:build
go run ./cmd/sops-aws-sync version
go run ./cmd/sops-aws-sync plan --help
go run ./cmd/sops-aws-sync sync --help
```

## Safety model

- Desired state always comes from committed Git blobs, never the working tree.
- Plaintext must be one strict top-level JSON object and is stored as RFC 8785 canonical JSON.
- Same-name secrets outside the exact reserved ownership tags fail closed.
- Default logs and reports omit secret values, paths, secret names, provider identifiers, and raw service errors.
- AWS authentication comes only from the standard AWS SDK credential and Region chain.
- Phase 2 mutates only `create` and `update`; unsupported lifecycle states remain non-mutating.

The CLI configuration contract is available through `plan --help` and `sync --help`. Flags override `SOPS_AWS_SYNC_*` environment variables, which override an optional explicit YAML configuration file, which override defaults.

## Release shape

The project is binary-only. GoReleaser builds Linux and macOS binaries for amd64 and arm64, plus checksums and SBOMs. Releases use the isolated `.github/workflows/attest.yml` workflow for GitHub artifact provenance. No container image is published.

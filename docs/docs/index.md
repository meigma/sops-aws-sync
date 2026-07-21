---
title: sops-aws-sync
---

# sops-aws-sync

`sops-aws-sync` reconciles committed SOPS-encrypted JSON documents with an
explicitly owned AWS Secrets Manager namespace. One selected `.sops.json` file
maps deterministically to one canonical `SecretString`.

The Go CLI owns Git reads, in-process SOPS decryption, AWS observation,
planning, mutation, and verification. The Node 24 Action is a thin adapter that
installs a checksum- and provenance-verified exact CLI release, constructs a
closed argument list, invokes the CLI without a shell, and publishes only the
validated redacted report.

Start with:

- [Configuration](configuration.md) for the CLI contract and precedence;
- [GitHub Action](github-action.md) for trusted workflow use and OIDC;
- [Operations and security](operations-and-security.md) for IAM, KMS,
  consistency, interruption, logging, and recovery; and
- [Release verification](release-verification.md) before executing a release
  artifact.

## Reconciliation boundary

Desired state is the tree of one resolved Git commit, never the working tree,
index, or untracked files. Selected documents must be regular `.sops.json`
blobs. Every document must decrypt with a valid SOPS MAC and become exactly one
top-level JSON object in the RFC 8785 canonicalization profile.

The tool creates missing owned secrets, updates changed values, restores
reintroduced secrets, and schedules removed secrets for deletion with a
recovery window. It never adopts a foreign same-name secret, force-deletes a
secret, manages rotation or replication, or provisions IAM, OIDC, or KMS.

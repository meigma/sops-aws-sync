---
title: Release verification
---

# Release verification

One immutable release pairs the Action source and committed bundle with Linux
and macOS binaries for amd64 and arm64. The release also contains
`checksums.txt`, per-binary SBOMs, and GitHub-hosted provenance attestations.

Download the exact asset and checksum file for the release. For example, on
Linux amd64:

```sh
gh release download v0.1.1 \
  --repo meigma/sops-aws-sync \
  --pattern sops-aws-sync_0.1.1_linux_amd64 \
  --pattern checksums.txt

sha256sum --check checksums.txt --ignore-missing
```

Then verify GitHub build provenance against the isolated signer workflow and
immutable release ref:

```sh
gh attestation verify ./sops-aws-sync_0.1.1_linux_amd64 \
  --repo meigma/sops-aws-sync \
  --signer-workflow meigma/sops-aws-sync/.github/workflows/attest.yml \
  --source-ref refs/tags/v0.1.1 \
  --deny-self-hosted-runners
```

`checksums.txt` detects corruption; provenance authenticates the repository,
workflow identity, release ref, and artifact digest. The Action enforces both
checks on downloads and tool-cache hits before executing the binary with the
caller's AWS identity.

For a private repository, authenticate `gh` with a token that can read contents
and attestations before downloading or verifying.

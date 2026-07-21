# How to verify a downloaded release binary

Confirm that a directly downloaded `sops-aws-sync` CLI binary matches the
published checksums and was built by the project's own release pipeline,
before you run it with AWS access.

Use this guide when you install the binary yourself — on a bastion host, in
non-GitHub CI, from an air-gapped mirror, or for an audit — rather than
through the GitHub Action. The Action performs this same checksum and
provenance verification automatically on every install and on every tool-cache
hit; this manual procedure is only for direct downloads.

For *why* provenance verification matters and what an attestation does and
does not prove, see
[About the security and trust model](../explanation/security-and-trust.md).

## Prerequisites

- A downloaded release binary and its `checksums.txt`, both from one specific
  release tag `v<version>` of `meigma/sops-aws-sync`.
- The [GitHub CLI (`gh`)](https://github.com/cli/cli#installation) installed
  and new enough to expose the attestation policy flags used below, and
  authenticated (`gh auth login`). Installing `gh` is out of scope.
- For a **public** repository, a normally authenticated `gh` verifies
  anonymously against the public attestation. For a **private** repository, the
  authenticated account or `GH_TOKEN` must be able to read the repository and
  its attestations (`contents: read` plus attestation read access).
- A shell with `sha256sum` (Linux) or `shasum -a 256` (macOS).

Releases are published only for Linux and macOS on `amd64` and `arm64`. For
the full supported-platform list, see the
[configuration reference](../reference/configuration.md).

## Steps

### 1. Identify and download the platform asset

Release assets are named
`sops-aws-sync_<version>_<os>_<arch>`, where `<version>` is the release version
with **no** leading `v`, `<os>` is one of `linux` or `darwin`, and `<arch>` is
one of `amd64` or `arm64`. The asset is the raw executable — there is no
archive to unpack. The release also publishes a single `checksums.txt`
manifest covering every asset.

Set the release identity for the commands that follow:

    VERSION=1.2.3            # the release version, no leading v
    OS=linux                # linux or darwin
    ARCH=amd64              # amd64 or arm64
    ASSET="sops-aws-sync_${VERSION}_${OS}_${ARCH}"

Download the binary and the checksum manifest for that tag:

    gh release download "v${VERSION}" \
      --repo meigma/sops-aws-sync \
      --pattern "${ASSET}" \
      --pattern checksums.txt

### 2. Confirm the checksum against the published manifest

Recompute the binary's SHA-256 and confirm it matches the manifest line for
your asset. Each `checksums.txt` line is `<64-hex-digest>␣␣<asset-name>` (two
spaces).

On Linux:

    grep "  ${ASSET}$" checksums.txt | sha256sum -c -

On macOS:

    grep "  ${ASSET}$" checksums.txt | shasum -a 256 -c -

Expected output:

    sops-aws-sync_1.2.3_linux_amd64: OK

If this prints `FAILED` or the asset is absent from `checksums.txt`, stop — see
[If verification fails](#if-verification-fails).

### 3. Verify provenance with `gh attestation verify`

Confirm the binary carries a signed provenance attestation from the project's
release pipeline by verifying it against the pinned policy. `gh` computes the
binary's digest and requires a signed SLSA provenance attestation that binds
it.

    gh attestation verify "${ASSET}" \
      --repo meigma/sops-aws-sync \
      --signer-workflow meigma/sops-aws-sync/.github/workflows/attest.yml \
      --source-ref "refs/tags/v${VERSION}" \
      --cert-oidc-issuer https://token.actions.githubusercontent.com \
      --deny-self-hosted-runners \
      --predicate-type https://slsa.dev/provenance/v1 \
      --digest-alg sha256

Each flag pins one property of a trustworthy build:

| Flag | Required value |
|------|----------------|
| `--repo` | `meigma/sops-aws-sync` |
| `--signer-workflow` | `meigma/sops-aws-sync/.github/workflows/attest.yml` |
| `--source-ref` | `refs/tags/v<version>` |
| `--cert-oidc-issuer` | `https://token.actions.githubusercontent.com` |
| `--deny-self-hosted-runners` | (no value) |
| `--predicate-type` | `https://slsa.dev/provenance/v1` |
| `--digest-alg` | `sha256` |

Expected output ends with a success line such as:

    Loaded digest sha256:… for file://sops-aws-sync_1.2.3_linux_amd64
    ✓ Verification succeeded!

The attestation binds the binary directly: `gh` accepts the artifact only when
a verified statement carries the SLSA provenance predicate and lists your
binary's exact SHA-256 as a subject. Verify the **binary**, not
`checksums.txt`.

!!! note "Pinning to the exact release commit"
    For a stronger pin to the exact release commit, add
    `--source-digest <commit>` and `--signer-digest <commit>`, using the
    commit the tag resolves to (from the release page or
    `gh release view v${VERSION} --repo meigma/sops-aws-sync`). The policy
    above is already sufficient on its own; for why pinning the commit adds
    defense in depth, see
    [About the security and trust model](../explanation/security-and-trust.md).

### 4. Confirm the embedded version

Only after **both** checks pass, make the binary executable and confirm its
embedded build metadata matches the release you verified:

    chmod +x "${ASSET}"
    "./${ASSET}" version

Expected output has the form:

    sops-aws-sync <version> (<commit>) built <date>

The `version` subcommand loads no configuration and touches no AWS. Confirm
`<version>` matches `VERSION` (the no-leading-`v` version you set) and
`<commit>` matches the release commit.

## If verification fails

Any mismatch means **do not run the binary** — fetch a clean copy and start
over. Common causes:

### The checksum does not match

**Cause:** The binary does not match the release's published `checksums.txt` —
a corrupted download, the wrong asset, or a tampered file.

**Solution:** Re-download the asset and `checksums.txt` for the same tag and
repeat step 2.

### `gh attestation verify` reports no matching attestation, a signer mismatch, or a source-ref mismatch

**Cause:** The binary lacks a valid provenance attestation signed by
`meigma/sops-aws-sync/.github/workflows/attest.yml` for that tag. A binary from
a fork, a re-signed artifact, or one built after a tag was moved or deleted
fails this by design.

**Solution:** Discard the binary. Download only from the official
`meigma/sops-aws-sync` releases and re-run step 3. Do not weaken the policy
flags to make verification pass.

### The attestation was signed on a self-hosted runner

**Cause:** `--deny-self-hosted-runners` rejects any attestation produced off
GitHub-hosted infrastructure. The project's releases are built on
GitHub-hosted runners, so this indicates an untrusted build.

**Solution:** Discard the binary; obtain an official release.

### `gh: command not found`, or `gh` lacks the policy flags

**Cause:** `gh` is missing or too old to support the attestation policy flags.

**Solution:** Install or update the
[GitHub CLI](https://github.com/cli/cli#installation), then re-run step 3.

## Related

- [About the security and trust model](../explanation/security-and-trust.md)
  — why provenance is mandatory and what the attestation proves.
- [How to deploy sops-aws-sync with GitHub Actions](deploy-with-github-actions.md)
  — where this verification happens automatically on every install.
- [Configuration reference](../reference/configuration.md)
  — supported platforms, the `version` output format, and the Action inputs
  that drive automatic verification.

# YAML-Encoded SOPS Support Plan

## Goal

Add `.sops.yaml` and `.sops.yml` as supported desired-state inputs alongside
`.sops.json`, without changing reconciliation or AWS behavior.

The input contract should be:

- read every supported file from the exact Git commit, never the working tree;
- require one top-level object/mapping per file and a valid SOPS MAC;
- accept only JSON-compatible YAML values, then store the existing RFC 8785
  canonical JSON representation as `SecretString`;
- preserve source ownership when a file keeps its path stem but changes between
  `.sops.json`, `.sops.yaml`, and `.sops.yml`; and
- fail the complete desired-state build before AWS access on invalid or
  ambiguous input.

## Delivery slices

### 1. Prove YAML decryption and canonicalization

- Add one hermetic encrypted YAML fixture using the existing test age key.
- Generalize the application decryption port from JSON-only to a document
  decrypter that receives the selected source encoding.
- In `internal/adapters/sopsdecrypt`, select SOPS `formats.Json` or
  `formats.Yaml`. For YAML, parse exactly one document, reject duplicate or
  non-string keys and non-JSON values, convert it to JSON, and reuse the current
  strict JCS path and size limits.
- Prove equivalent JSON and YAML objects produce identical canonical bytes;
  retain the existing cancellation, MAC, limit, and redaction behavior.

### 2. Wire committed-source discovery and mapping

- Teach `internal/adapters/gitrepo` to select all three suffixes and carry the
  encoding on `application.EncryptedDocument`.
- Teach `internal/domain` path mapping to strip each exact suffix. Normalize
  YAML suffixes to the existing `.sops.json` identity before hashing so an
  encoding-only rename does not conflict with an already managed secret.
- Add table-driven coverage for mixed-format exact-commit discovery, both YAML
  suffixes, matching non-regular entries, and a cross-format duplicate stem.

### 3. Finish the public contract

- Update JSON-only wording in CLI help, Action metadata, README/operator docs,
  and repository descriptions; include one small YAML example.
- Keep configuration, reports, Action inputs/bundle behavior, AWS adapters, and
  reconciliation policy unchanged.
- Run `git diff --check` and the complete `mise exec -- moon run root:check`
  gate. No live AWS test is required for this source-format-only change.

## Acceptance checks

- Existing `.sops.json` behavior remains backward compatible.
- `.sops.yaml` and `.sops.yml` each map one committed file to the expected
  secret name and canonical JSON value.
- Converting an existing source between supported encodings preserves its
  ownership identity; changing its directory or stem does not.
- Multiple YAML documents, non-mapping roots, duplicate/non-string keys,
  unsupported YAML values, invalid MACs, and cross-format name collisions fail
  before any AWS mutation.

## Out of scope

Raw YAML storage or formatting preservation, dotenv/INI/binary inputs,
content-based format detection, and changes to AWS lifecycle behavior.

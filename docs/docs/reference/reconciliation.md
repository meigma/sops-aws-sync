# Reconciliation reference

This reference states the rules that turn committed files into a plan: how
source documents are selected and validated, how their paths become AWS secret
names, how desired state combined with observed state produces each decision, in
what order operations run, which conditions block a plan, and which AWS
Secrets Manager operations the tool issues.

It is descriptive only. The reasoning behind these rules is in
[About the reconciliation model](../explanation/reconciliation-model.md) and
[About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).
Exit codes, run statuses, and the report schema are in
[Results and exit codes reference](results.md). Flags, environment variables,
defaults, and precedence are in
[Configuration reference](configuration.md).

## Source selection

Desired state is built from committed content at one resolved Git commit. The
working tree and the index are never consulted.

- Only regular Git blobs whose path ends in one of three literal,
  case-sensitive suffixes, located under the configured source root, are
  selected. Discovery is recursive through the source-root subtree. The matched
  suffix fixes the document's encoding, and there is no flag to override it:

  | Suffix | Encoding |
  |--------|----------|
  | `.sops.json` | JSON |
  | `.sops.yaml` | YAML |
  | `.sops.yml` | YAML |

  The two YAML suffixes are equivalent and carry one identical encoding
  contract.
- Paths that do not end in one of the three suffixes are ignored, not errors.
  Files such as `config.yaml`, `.yml`, `README.md`, `.enc.json`, and a plain
  `.json` that is not `.sops.json` are silently skipped.
- A selected path — one that matches a supported suffix — whose tree entry is
  not a regular blob (a symlink, submodule, or any other non-regular mode) is a
  hard load error that aborts the run.
- Each selected blob must be no larger than the `max-encrypted-bytes` limit (see
  [Configuration reference](configuration.md)). The blob size is checked before
  the blob is read.
- When the source root is not `.` and its directory does not exist in the
  committed tree, the load fails as a missing source root.

## Plaintext contract

After a blob is selected, it is decrypted and its plaintext must satisfy every
condition below. Any failure aborts the run before observation or mutation.

Both formats:

- The SOPS document MAC is verified during decryption.
- The decrypted plaintext is valid UTF-8.
- The value is re-encoded to RFC 8785 (JCS) canonical JSON. A YAML plaintext is
  first converted to intermediate JSON, then passed through the same validator
  and canonicalizer as a JSON source, so both encodings emit byte-identical
  canonical JSON for equivalent content.
- The canonical value is non-empty and no larger than 65,536 bytes.

JSON sources also:

- The plaintext is exactly one top-level JSON object. Bare scalars, top-level
  arrays, and multiple top-level values are rejected.
- No member name is duplicated at any nesting depth.
- No data follows the top-level object.

YAML sources also:

- The plaintext is exactly one YAML document; a second `---` document is
  rejected.
- The top-level value is a mapping. Bare scalars and top-level sequences are
  rejected.
- Every mapping key, at any nesting depth, is a string. Non-string keys — such
  as integer, boolean, or null keys — are rejected.
- No mapping key is duplicated within a mapping, at any nesting depth.
- Anchors and aliases are rejected.
- Every scalar resolves to a JSON scalar type: null, string, boolean, integer,
  or float. Other resolved tags — including YAML timestamps such as
  `2026-07-21` and explicitly `!!binary` scalars — are rejected. An integer
  outside the JSON integer profile, and a non-finite float (`.inf`, `-.inf`,
  `.nan`), are also rejected.
- The supported structure is one top-level mapping whose values may be nested
  mappings with string keys, sequences, and the allowed scalars.

## Name mapping

The AWS secret name is derived from the source path; it is never read from the
document or stored separately.

```
secret name = secret-prefix + "/" + (source path − source-root − matched suffix)
```

The secret prefix has any single trailing `/` trimmed before it is joined. The
relative stem is the repository-relative path with the source-root prefix and
whichever supported suffix matched (`.sops.json`, `.sops.yaml`, or `.sops.yml`)
removed. Because all three suffixes strip to the same stem, `db.sops.json`,
`db.sops.yaml`, and `db.sops.yml` at one location derive the same secret name.
Case is preserved and no character is substituted. An empty stem fails mapping.

Worked example, with source root `secrets` and secret prefix `/acme/payments`:

```
secrets/production/database.sops.json  →  /acme/payments/production/database
```

- Moving or renaming a document changes both the derived name and the source
  identity, so the tool sees a delete of the old name and a create of the new
  name, never a rename.
- Two documents in a single snapshot that map to the same name is a hard load
  error. This includes two files that share one stem but differ only in
  encoding (for example `value.sops.json` and `value.sops.yml`), since both
  derive the same name.
- A path that would yield a character outside the name character set (below)
  fails mapping.

## Name constraints

A derived secret name must satisfy every constraint below.

| Constraint | Value |
|------------|-------|
| Non-empty | Required |
| Maximum length | 512 bytes |
| Allowed characters | `A`–`Z`, `a`–`z`, `0`–`9`, and `/ _ + = . @ -` |

## Ownership tags

Ownership is recorded in three reserved AWS tags. The keys and values are fixed
and are not configurable.

| Tag key | Value |
|---------|-------|
| `sops-aws-sync:managed-by` | The literal `sops-aws-sync`. |
| `sops-aws-sync:scope` | Lowercase-hex SHA-256 over the label `sops-aws-sync:scope:v1`, the repository-id, the cleaned source-root, and the trailing-slash-trimmed secret-prefix, joined with NUL (`\x00`) separators. |
| `sops-aws-sync:source` | Lowercase-hex SHA-256 of the cleaned repository-relative source path, with any `.sops.yaml` or `.sops.yml` suffix first normalized to `.sops.json`. The hashed path includes the source-root prefix. |

- A secret is owned only when both `managed-by` and `scope` match the current
  scope exactly. The `source` tag then identifies which committed path owns it.
- The reserved tags are written only by the `CreateSecret` request. There is no
  post-create tag-repair path.
- Non-reserved tags are neither read for decisions nor modified.
- Changing only a document's encoding (`.sops.json` ↔ `.sops.yaml` ↔
  `.sops.yml`) at the same location leaves both the derived name and the
  `source` tag unchanged, so ownership is preserved and, because the two
  encodings produce identical canonical JSON, no value drift arises. Changing
  the stem, the directory within the root, or the source-root prefix changes
  the source identity.

The re-scoping consequences of these rules are covered in
[About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).

## Decision matrix

A plan is the union of desired names and tag-discovered scope members. Each
entry resolves to exactly one decision from the classified observed state.

For a name present in the committed desired set:

| Observed classification | Decision |
|-------------------------|----------|
| Absent (no secret) | Create |
| Owned, active, string value byte-equal to the desired canonical JSON | Unchanged |
| Owned, active, string value that differs | Update |
| Owned, active, binary value | Update |
| Owned, active, no AWSCURRENT version | Update |
| Owned, scheduled for deletion | Restore, then at most one follow-up Update |
| Not owned, or an unsupported state | Conflict |

For a name owned within the scope but absent from the committed desired set:

| Observed classification | Decision |
|-------------------------|----------|
| Owned, active (string, binary, discovery-only — a scope member whose payload was never read — or without an AWSCURRENT version) | ScheduleDeletion |
| Owned, scheduled for deletion | Unchanged |
| Not owned, or an unsupported state | Conflict |

`Unchanged` is summary metadata, not an executed operation. Value comparison is
a byte-for-byte match of the desired canonical JSON against the stored string
value; a stored value that is not byte-identical canonical JSON produces an
Update even when it is semantically equal.

## Operation phase order

Executable operations run in a fixed phase order. Within a phase, operations are
ordered by ascending secret name.

1. Restore
2. Create
3. Update
4. ScheduleDeletion

A name observed as owned-scheduled but still desired is restored in phase 1,
then reconciled to the committed value by a single bounded follow-up cycle. That
cycle performs one Update only when the restored value is not already byte-equal
to the desired canonical JSON; when it already matches, the restore alone
converges the name.

## Empty-desired-state gate

When the committed desired set is empty **and** the plan contains at least one
ScheduleDeletion **and** `allow-empty` is false, the run is rejected with status
`invalid` and performs zero mutations.

- Authorization through `allow-empty` applies to the single transition. Once the
  secrets are scheduled for deletion they classify as owned-scheduled and
  produce no further ScheduleDeletion, so later runs converge without
  `allow-empty`.
- Deletions from a non-empty desired set are not gated.

An empty source root is distinct from a missing source root. Git cannot track an
empty directory: deleting every tracked file under the source root removes the
directory itself from the committed tree. Such a run is rejected as a missing
source root (status `invalid`, exit code 3; see
[Results and exit codes reference](results.md)) during source selection, before
the empty-desired-state gate is ever evaluated. An intentionally empty source
root therefore requires a tracked placeholder file — for example
`secrets/.gitkeep` — to keep the directory present in the committed tree, at
which point the empty-desired-state gate applies as described above.

## Conflict classes

A conflict is a non-sensitive classification. Any single conflict blocks the
entire plan before any mutation is performed.

| Class | Triggering condition |
|-------|----------------------|
| `ownership` | The same-name secret carries no reserved tags, or its `managed-by`/`scope` tags do not both match the scope. |
| `source` | The secret is owned (`managed-by` and `scope` match) but its `source` tag does not match the desired document's source identity. |
| `source-tag` | The `source` tag is present but is not a valid SHA-256 digest. |
| `service-owned` | The secret has a non-empty owning service (its `OwningService`, or its `Type`). |
| `rotation` | Rotation is enabled, or an `AWSPENDING` version indicates a rotation in progress. |
| `replication` | The secret has one or more replica Regions. |
| `staging` | Staging is ambiguous: more than one `AWSCURRENT` version, or the retrieved current version's stage disagrees. |
| `payload` | The current version exists but is neither a clean string nor a clean binary value (both are set, or the expected value is absent). |

The run-level exit code for a conflicting plan is in
[Results and exit codes reference](results.md). Remediation for each class is in
[How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md).

## Deletion behavior

- Deletion is always a scheduled deletion. The configured
  `recovery-window-days` value (see [Configuration reference](configuration.md))
  is passed verbatim to AWS as the recovery window.
- Force deletion (`ForceDeleteWithoutRecovery`) is never requested.

## AWS Secrets Manager operations

The tool issues only the operations below. They are the basis for the IAM policy
in
[How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md).

| Operation | Role |
|-----------|------|
| `ListSecrets` | Scope discovery. Filtered on tag key `sops-aws-sync:managed-by` with value `sops-aws-sync`, with `IncludePlannedDeletion=true`, paginated. Scope-digest filtering is applied in the tool after listing, not by the filter. |
| `DescribeSecret` | Direct metadata observation of a specific name. |
| `GetSecretValue` | Reads the `AWSCURRENT` value of an owned active secret by version id and stage. |
| `CreateSecret` | Creates an absent secret with its canonical value, the three reserved ownership tags, and an idempotency token. |
| `PutSecretValue` | Writes a new `AWSCURRENT` version (Update) with an idempotency token. |
| `RestoreSecret` | Cancels a scheduled deletion for an owned name. |
| `DeleteSecret` | Schedules deletion with the recovery window (ScheduleDeletion, phase 4). |

No standalone `TagResource` call is made; the reserved tags are carried on the
`CreateSecret` request. The IAM permission this nonetheless requires is covered
in
[How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md).

## SOPS integration

- Decryption is delegated to getsops with the format selected per document from
  its path suffix — JSON for `.sops.json`, YAML for `.sops.yaml` and
  `.sops.yml` — and the document MAC is verified for either format.
- Key backends (age, KMS, PGP, Vault) are whatever SOPS resolves from its own
  environment and each document's metadata. The tool contributes no key
  configuration and layers the stricter content contract above on top of the
  SOPS result.

## Deliberate exclusions

| Content | Owning document |
|---------|-----------------|
| Why these rules exist and the re-scoping trap | [About the reconciliation model](../explanation/reconciliation-model.md), [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md) |
| How to resolve a specific conflict | [How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md) |
| How to turn the operations list into an IAM policy | [How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md) |
| Flag defaults, such as the recovery-window range and the `max-encrypted-bytes` default | [Configuration reference](configuration.md) |
| Exit codes, run statuses, verification values, and the report schema | [Results and exit codes reference](results.md) |

## See also

- [About the reconciliation model](../explanation/reconciliation-model.md)
- [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md)
- [How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md)
- [How to diagnose and recover from a failed run](../how-to/diagnose-and-recover.md)
- [Configuration reference](configuration.md)
- [Results and exit codes reference](results.md)

---
title: Operations and security
---

# Operations and security

## AWS permissions

The runtime role needs `secretsmanager:ListSecrets` for scope discovery and the
following operations for secrets under the configured name prefix:

- `DescribeSecret`;
- `GetSecretValue`;
- `CreateSecret`;
- `TagResource` for reserved ownership tags created with a new secret;
- `PutSecretValue`;
- `DeleteSecret` with a recovery window; and
- `RestoreSecret`.

Apply resource restrictions wherever Secrets Manager's IAM semantics permit.
The tool does not need IAM, OIDC-provider, or KMS administration permissions.
Limit KMS access to keys required to decrypt the selected SOPS documents and,
when configured, the customer-managed Secrets Manager key.

The tool does not provision this role, GitHub's OIDC provider, KMS keys, or
Secrets Manager rotation and replication configuration.

## Ownership and destructive operations

A secret is managed only when its reserved `managed-by` and `scope` tags match
exactly and its `source` tag is a valid source-path digest. A desired same-name
secret must also have the expected source identity. Foreign, malformed,
rotation-managed, service-owned, replicated, or ambiguous staging state is a
conflict and prevents mutation for that plan.

Removed managed sources are scheduled for deletion after restore, create, and
update operations. The recovery window defaults to 30 days and can be set from
7 through 30 days. Force deletion is not implemented. An empty desired set must
be authorized with `--allow-empty` only when it would add deletion operations.

## Consistency and concurrency

Secrets Manager list results are eventually consistent and may lag by up to
five minutes. The CLI combines fully paginated discovery with direct
`DescribeSecret` and `GetSecretValue` calls for desired and affected names.
Success means that observed union re-plans to zero executable operations and
zero conflicts before the verification deadline; it is not a globally strongly
consistent snapshot.

Use exactly one active writer for each ownership scope. In GitHub Actions, set a
non-canceling concurrency group unique to that scope. The CLI checks
preconditions before each operation, but Secrets Manager does not provide a
multi-secret transaction or general compare-and-swap primitive.

## Interruption, failure, and recovery

Every AWS call is context-bound and has a shorter operation deadline. The AWS
SDK owns bounded request retries. A lost response to a mutation is treated as
ambiguous: the CLI re-observes that exact name and never performs an unproven
blind write retry.

SOPS's stable Go decrypt binding is not context-aware. Decryption runs in a
single-use worker. Cancellation is terminal at the process boundary: the CLI
returns interruption status without waiting for an in-flight provider call,
and process termination ends that call. No AWS mutation starts until all
selected documents decrypt and validate.

Completed AWS operations are not rolled back. On partial failure the CLI stops
starting new operations, emits safe completion counts, and exits nonzero. A
later invocation resumes from observed state. Scheduled secrets remain
restorable until their AWS recovery window expires.

## Logging and reports

Operational logs go to stdout. Cobra usage and an emergency logger startup
failure go to stderr. Machine results go only to `--report-file`.

Default and debug logs may contain operation kinds, counts, durations, safe AWS
error codes, request IDs, and run-local resource ordinals. They do not contain
plaintext or ciphertext documents, JSON keys, credentials, tokens, KMS
identifiers, provider identifiers, raw SDK/SOPS errors, secret names, or source
paths. `--show-resource-names` changes only identifier visibility; it never
permits secret values.

Decrypted bytes remain in process memory and are never written to disk, placed
in environment variables, passed on the command line, or included in reports.
The Go runtime and AWS SDK do not provide a reliable memory-zeroization
guarantee.

# How to grant sops-aws-sync least-privilege AWS access

Provision the prefix-scoped AWS Secrets Manager and KMS permissions the tool
needs, and attach them to an identity the tool can consume.

!!! note

    The tool accepts **no** credential inputs. Credentials resolve only from the
    standard AWS SDK chain (environment variables, shared config/profile, SSO,
    web identity such as IRSA/OIDC, and container or instance roles). Only
    `--aws-region` / `--aws-profile` steer resolution, and the Action forwards
    only `--aws-region`. For why authentication is the caller's responsibility,
    see [About the security and trust model](../explanation/security-and-trust.md).

## Prerequisites

- Permission to author IAM policies and attach them to a role or user.
- The name of the KMS key(s) SOPS uses to decrypt your documents (each file's
  own SOPS metadata names them) and of any customer-managed key that encrypts
  the target Secrets Manager secrets.
- A chosen, frozen ownership scope (`repository-id`, `source-root`,
  `secret-prefix`). Treat these as an immutable identity — see
  [About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).

## Steps

### 1. Grant the Secrets Manager operations

The tool issues exactly seven Secrets Manager operations. Grant one IAM action
per operation:

- **Discovery (account-level):** `secretsmanager:ListSecrets`. This operation
  does not support resource-level permissions, so it must be granted on
  `Resource: "*"`.
- **Per-secret lifecycle (prefix-scoped in step 4):**
  `secretsmanager:DescribeSecret`, `secretsmanager:GetSecretValue`,
  `secretsmanager:CreateSecret`, `secretsmanager:PutSecretValue`,
  `secretsmanager:RestoreSecret`, `secretsmanager:DeleteSecret`.

Grant `DeleteSecret` for teardown; the tool never issues a separate force-delete
call. For why deletion is soft and reversible, see
[About the reconciliation model](../explanation/reconciliation-model.md). For the
authoritative operation catalog and why the tool issues each call, see the
[Reconciliation reference](../reference/reconciliation.md).

### 2. Add `secretsmanager:TagResource`

Include `secretsmanager:TagResource` in the per-secret statement. The tool makes
no standalone `TagResource` API call — the three ownership tags ride the
`CreateSecret` request — but AWS requires the `secretsmanager:TagResource`
permission to create a secret *with* tags. Grant it, or every `CreateSecret`
fails.

The reserved tags are written at create time only; there is no post-create
tag-repair path, so a secret created without these permissions cannot later be
adopted. See
[About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md)
for the operational consequence.

### 3. Grant KMS where your keys require it

KMS access is conditional on how your documents and secrets are encrypted:

- **`kms:Decrypt`** is needed when a source document references an AWS KMS key
  in its own metadata (SOPS calls KMS in-process to decrypt it), and when a
  Secrets Manager secret is encrypted with a customer-managed key (AWS calls KMS
  server-side to read it).
- **`kms:GenerateDataKey`** is needed to write a new secret value under a
  customer-managed key.

If every target secret uses the AWS-managed key `aws/secretsmanager`, you need
no customer-managed-key grant. If none of your documents use AWS KMS (for
example, they use age), you need no SOPS-key grant, and you can drop the KMS
statement entirely. The tool needs no IAM, OIDC-provider, or KMS administration
permissions. See the AWS documentation on
[permissions for a customer managed key](https://docs.aws.amazon.com/secretsmanager/latest/userguide/security-encryption.html)
for the general Secrets Manager–KMS relationship.

### 4. Scope every resource ARN to your prefix

AWS appends a random six-character suffix to every secret's ARN (for example
`-a1b2c3`), so you cannot know a secret's full ARN before it exists. Scope the
per-secret statement to the prefix path followed by `/*`; the trailing `*`
matches both the rest of the name and the appended suffix. If you ever scope a
statement to one specific secret instead of the whole prefix, append `-??????`
(six literal `?`) or `-*` to that secret's name so the pattern still matches its
suffix.

The complete least-privilege policy, for prefix `/acme/payments` in
`us-east-1`, account `123456789012`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SopsAwsSyncDiscover",
      "Effect": "Allow",
      "Action": "secretsmanager:ListSecrets",
      "Resource": "*"
    },
    {
      "Sid": "SopsAwsSyncLifecycle",
      "Effect": "Allow",
      "Action": [
        "secretsmanager:DescribeSecret",
        "secretsmanager:GetSecretValue",
        "secretsmanager:CreateSecret",
        "secretsmanager:PutSecretValue",
        "secretsmanager:RestoreSecret",
        "secretsmanager:DeleteSecret",
        "secretsmanager:TagResource"
      ],
      "Resource": "arn:aws:secretsmanager:us-east-1:123456789012:secret:/acme/payments/*"
    },
    {
      "Sid": "SopsAwsSyncKms",
      "Effect": "Allow",
      "Action": [
        "kms:Decrypt",
        "kms:GenerateDataKey"
      ],
      "Resource": [
        "arn:aws:kms:us-east-1:123456789012:key/SOPS-DOCUMENT-KEY-ID",
        "arn:aws:kms:us-east-1:123456789012:key/SECRETS-MANAGER-CMK-ID"
      ]
    }
  ]
}
```

Substitute your region, account ID, and prefix throughout, and adjust the ARN
partition (`aws-us-gov`, `aws-cn`) if you are not in the commercial partition.
Drop the `SopsAwsSyncKms` statement or its individual key ARNs where step 3 says
they are unnecessary.

### 5. Deliver credentials to the tool

Attach the policy to whichever identity the standard AWS SDK chain resolves at
run time:

- **GitHub Actions:** assume a role via AWS OIDC in a step that runs *before*
  the Action. Continue with
  [How to deploy sops-aws-sync with GitHub Actions](deploy-with-github-actions.md).
- **A bastion or local admin:** use a shared profile, SSO session, or instance
  role. Select a profile with `--aws-profile` and a region with `--aws-region`
  when the environment does not already supply them; see the
  [Configuration reference](../reference/configuration.md) for their defaults.

A region must resolve — through `--aws-region`, `AWS_REGION` /
`AWS_DEFAULT_REGION`, the profile, or instance metadata — or setup fails before
any AWS call.

## Verify access

Run a read-only `plan` against a scratch prefix. `plan` never mutates anything,
so it is safe to point at any scope:

```console
$ sops-aws-sync plan \
    --repository-id you/demo \
    --secret-prefix /scratch/demo \
    --aws-region us-east-1
```

The reliable success signal is the run completing without AWS permission errors:
exit 0 and no `AccessDenied` or normalized error metadata in the JSON logs. The
status itself may legitimately be `drift` — against a scratch scope that owns
nothing yet, every committed document classifies as a pending create, and
`drift` confirms discovery and credentials work just as well as `converged`
does. Only a scope whose `--source-root` holds no committed source documents
reports `converged`.

Because `plan` is read-only, it verifies discovery (`ListSecrets`), region
resolution, and the credential chain — plus read access on any secrets the scope
already owns. It cannot confirm the write-side grants (`CreateSecret`,
`PutSecretValue`, create-time `TagResource`, `kms:GenerateDataKey`); only a real
`sync` exercises those. For what each status means, see the
[Results and exit codes reference](../reference/results.md).

## Troubleshooting

### The run fails before any AWS call, with status `invalid` (exit 3)

**Solution:** No region resolved; the JSON logs carry the message
`AWS configuration is invalid`. Set `--aws-region`, export `AWS_REGION`, or give
your profile a region. The tool refuses to start against an empty region.

### The run fails with status `apply-failed` (exit 5) and no operations ran

**Solution:** The credential chain produced nothing usable; the JSON logs carry
the message `AWS credential loading failed`. Confirm the SDK chain resolves
credentials on the runner (assume-role step, profile, SSO session, or instance
role) before invoking the tool.

### A single operation is denied part-way through a run

**Solution:** The JSON logs record the failed AWS operation and its error
`code` (for example `AccessDeniedException`) as normalized metadata. Grant the
specific action named for that operation on your prefix ARN, then re-run.
Depending on the phase, a denial surfaces as `observation-failed` or
`apply-failed` (both exit 5). See
[How to diagnose and recover from a failed run](diagnose-and-recover.md).

### The tool cannot see secrets you created earlier

**Solution:** This is a scope boundary, not a permissions problem. Secrets
outside the current `repository-id` / `source-root` / `secret-prefix` are
invisible by design. See
[About ownership, scope, and fail-closed safety](../explanation/ownership-and-scope.md).

## Related

- [How to deploy sops-aws-sync with GitHub Actions](deploy-with-github-actions.md)
- [Reconciliation reference](../reference/reconciliation.md) — the authoritative
  AWS operation catalog
- [Configuration reference](../reference/configuration.md) — region/profile
  flags and defaults
- [About the security and trust model](../explanation/security-and-trust.md) —
  why AWS auth is the caller's responsibility
- [How to diagnose and recover from a failed run](diagnose-and-recover.md)

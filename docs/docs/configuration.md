---
title: Configuration
---

# Configuration

The CLI has three commands: `plan`, `sync`, and `version`. `plan` observes and
reports without mutating AWS. `sync` applies the deterministic plan and succeeds
only after verification. `version` does not load configuration or AWS.

Configuration precedence is fixed, highest first:

1. command flags;
2. explicit `SOPS_AWS_SYNC_*` environment variables;
3. the YAML file selected with `--config`;
4. defaults.

The file is never discovered implicitly. Unknown keys, malformed values, and
invalid durations fail before side effects. AWS credentials remain owned by the
AWS SDK's standard chain and are not copied into tool configuration.

| Flag | Default | Contract |
| --- | --- | --- |
| `--config` | none | Optional explicit YAML configuration file. |
| `--repository` | `.` | Local Git repository to open. |
| `--revision` | `HEAD` | Commit-ish resolved by go-git. |
| `--repository-id` | required | Stable ownership identity, normally `owner/repo`. |
| `--source-root` | `secrets` | Directory containing `.sops.json`, `.sops.yaml`, or `.sops.yml` files. |
| `--secret-prefix` | required | Secrets Manager name prefix and ownership boundary. |
| `--max-encrypted-bytes` | `8MiB` | Per-file encrypted input limit. |
| `--allow-empty` | `false` | Authorize an empty desired set to add scheduled deletions. |
| `--aws-region` | SDK chain | Optional Region override. |
| `--aws-profile` | SDK chain | Optional shared-profile override for local use. |
| `--recovery-window-days` | `30` | Scheduled deletion recovery period, from 7 through 30. |
| `--run-timeout` | `10m` | Whole-run deadline. |
| `--operation-timeout` | `30s` | Deadline for each AWS call. |
| `--verification-timeout` | `2m` | Known-name stabilization deadline. |
| `--aws-max-attempts` | SDK standard | Optional bounded request attempts. |
| `--aws-max-backoff` | SDK standard | Optional bounded retry backoff. |
| `--log-level` | `info` | `debug`, `info`, `warn`, or `error`. |
| `--log-format` | `json` | `json` or `text`; the Action selects JSON. |
| `--show-resource-names` | `false` | Explicitly disclose secret names and source paths. |
| `--report-file` | none | Write the non-sensitive machine report to a file. |

Environment names replace hyphens with underscores, for example
`SOPS_AWS_SYNC_SECRET_PREFIX` and `SOPS_AWS_SYNC_OPERATION_TIMEOUT`.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | Plan completed, or sync verified convergence. |
| 2 | `plan --detailed-exit-code` found executable drift. |
| 3 | Invalid configuration, input, or desired state. |
| 4 | Ownership or observed-state conflict. |
| 5 | Apply failed or ended with an unknown or partial result. |
| 6 | Verification failed or remained inconclusive. |
| 130 | Interrupted by cancellation, SIGINT, or SIGTERM. |

The stable report contains schema and tool versions, the resolved Git revision,
status, operation counts, conflict count, verification result, and durations.
It omits secret material and resource identifiers by default.

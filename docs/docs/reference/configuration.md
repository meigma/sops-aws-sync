# Configuration reference

This reference catalogs every command-line flag, environment variable, GitHub
Action input, and Action output that `sops-aws-sync` accepts or emits, together
with defaults, types, validation constraints, precedence, and the
stdout/stderr/report stream contract. The Action forwards its inputs to the same
CLI, so both surfaces share one option table.

For what a run returns — exit codes, run statuses, verification values, and the
report schema — see [Results and exit codes reference](results.md). For how
committed files become secret names and how a plan is decided, see
[Reconciliation reference](reconciliation.md).

## Subcommands

| Command | Configuration | AWS access | Mutates | Description |
|---------|---------------|------------|---------|-------------|
| `plan` | Loaded and validated | Yes | No | Builds a redacted reconciliation plan without mutating AWS. Supports the plan-only `--detailed-exit-code` flag. |
| `sync` | Loaded and validated | Yes | Yes | Applies the plan and always verifies convergence. No skip-verification mode exists. |
| `version` | Not loaded | No | No | Prints the build line and exits. |
| (no subcommand) | Not loaded | No | No | Prints help and exits. |

Every command rejects unexpected positional arguments. The root command's
`--version` flag prints the same build line as the `version` subcommand.

## Options

The following options are accepted by the `plan` and `sync` subcommands. The
environment-variable column gives the exact bound name; the derivation rule is
below the table. The Action-input column names the corresponding Action input,
or `—` when the option has no Action input.

| Flag | Environment variable | Action input | Default | Type / validation |
|------|----------------------|--------------|---------|-------------------|
| `--repository` | `SOPS_AWS_SYNC_REPOSITORY` | — | `.` | string; non-empty after trimming; local Git repository path |
| `--revision` | `SOPS_AWS_SYNC_REVISION` | `revision` | `HEAD` | string; non-empty after trimming; committed revision (resolved to a full 40-hex commit SHA during reconciliation) |
| `--repository-id` | `SOPS_AWS_SYNC_REPOSITORY_ID` | `repository-id` | none (required) | string; required (non-empty after trimming). The Action validates the input as an `owner/repository` identity |
| `--source-root` | `SOPS_AWS_SYNC_SOURCE_ROOT` | `source-root` | `secrets` | string. The Action requires a canonical repository-relative POSIX path |
| `--secret-prefix` | `SOPS_AWS_SYNC_SECRET_PREFIX` | `secret-prefix` (required) | none (required) | string; a trailing `/` is trimmed; must be a valid secret name (character and length rules → [reconciliation](reconciliation.md)). An empty value is rejected |
| `--max-encrypted-bytes` | `SOPS_AWS_SYNC_MAX_ENCRYPTED_BYTES` | — | `8MiB` | byte size with an optional binary suffix; must be positive |
| `--allow-empty` | `SOPS_AWS_SYNC_ALLOW_EMPTY` | `allow-empty` | `false` | boolean |
| `--aws-region` | `SOPS_AWS_SYNC_AWS_REGION` | `aws-region` | none (SDK chain) | string; optional Region override |
| `--aws-profile` | `SOPS_AWS_SYNC_AWS_PROFILE` | — | none | string; optional shared-profile override |
| `--recovery-window-days` | `SOPS_AWS_SYNC_RECOVERY_WINDOW_DAYS` | `recovery-window-days` | `30` | integer; 7 through 30 inclusive |
| `--run-timeout` | `SOPS_AWS_SYNC_RUN_TIMEOUT` | `run-timeout` | `10m` | Go duration; greater than 0 |
| `--operation-timeout` | `SOPS_AWS_SYNC_OPERATION_TIMEOUT` | `operation-timeout` | `30s` | Go duration; greater than 0 |
| `--verification-timeout` | `SOPS_AWS_SYNC_VERIFICATION_TIMEOUT` | `verification-timeout` | `2m` | Go duration; greater than 0 |
| `--aws-max-attempts` | `SOPS_AWS_SYNC_AWS_MAX_ATTEMPTS` | — | `0` | integer; 0 or greater |
| `--aws-max-backoff` | `SOPS_AWS_SYNC_AWS_MAX_BACKOFF` | — | `0s` | Go duration; 0 or greater |
| `--log-level` | `SOPS_AWS_SYNC_LOG_LEVEL` | — | `info` | one of `debug`, `info`, `warn`, `error` (case-insensitive) |
| `--log-format` | `SOPS_AWS_SYNC_LOG_FORMAT` | — | `json` | one of `json`, `text` (case-insensitive) |
| `--show-resource-names` | `SOPS_AWS_SYNC_SHOW_RESOURCE_NAMES` | `show-resource-names` | `false` | boolean |
| `--report-file` | `SOPS_AWS_SYNC_REPORT_FILE` | — | none | string; optional report file path |
| `--config` | none (not bindable) | — | none | string; optional YAML configuration file path; flag only |
| `--detailed-exit-code` | none (not bindable) | — | `false` | boolean; accepted only by the `plan` subcommand |

### Environment variables

Each bindable flag is bound to `SOPS_AWS_SYNC_` followed by the flag name in
uppercase with every `-` and `.` replaced by `_`. All options above are bindable
except `--config` and `--detailed-exit-code`, which have no environment
variable.

### Precedence

For any option, the effective value is resolved in this order, highest first:

1. The command-line flag.
2. The `SOPS_AWS_SYNC_*` environment variable.
3. A key in the YAML file named by `--config`.
4. The built-in default.

Configuration-file keys use the flag name (for example `secret-prefix`). The
configuration is unmarshaled with exact-key matching: any unknown key or
type mismatch is rejected and the command exits `3` (see
[results](results.md)). Rejected values are never echoed.

### Value syntax

Some value formats cannot be inferred from the type alone.

- **Go durations** (`--run-timeout`, `--operation-timeout`,
  `--verification-timeout`, `--aws-max-backoff`) are composed from explicit unit
  tokens `ns`, `us`, `ms`, `s`, `m`, `h` — for example `10m`, `30s`, `1h30m`.
  Bare numbers such as `300` and unit words such as `5min` are rejected. The
  three run, operation, and verification timeouts must be greater than zero;
  `--aws-max-backoff` also accepts `0s`.
- **Byte size** (`--max-encrypted-bytes`) is a positive integer with an optional
  binary suffix `GiB`, `MiB`, `KiB`, or `B`. A bare integer is a byte count.
  Decimal suffixes such as `MB` or `GB` are rejected.
- **Recovery window** (`--recovery-window-days`) is an integer from 7 through 30
  inclusive.
- **Boolean Action inputs** accept exactly the lowercase spellings `true` or
  `false`; any other value is rejected. The CLI boolean flags additionally
  accept the standard flag boolean spellings.
- **`--log-level` and `--log-format`** are matched case-insensitively; values are
  lowercased before validation.

## GitHub Action

The Action installs a verified CLI release and forwards a shell-free argument
vector to it. The shared options in the table above are supplied through the
Action inputs listed there. The inputs below have no shared CLI flag.

### Action-only inputs

| Input | Default | Type / validation |
|-------|---------|-------------------|
| `mode` | `plan` | `plan` or `sync`; selects the subcommand |
| `fail-on-drift` | `false` | boolean; valid only when `mode: plan`; rejected in sync mode |
| `cli-version` | release-managed exact version (currently `0.1.1`) | exact semantic version without a leading `v` |
| `github-token` | `${{ github.token }}` | string; must be non-empty; masked; authenticates release download and attestation verification only |

### Forwarded and fixed arguments

The Action always sets `--repository` to `GITHUB_WORKSPACE` and always appends
`--log-format=json` and a temporary `--report-file`. It appends
`--detailed-exit-code` only when `mode: plan` and `fail-on-drift: true`. Inputs
left empty are omitted from the argument vector. The Action exposes no input for
`--max-encrypted-bytes`, `--aws-profile`, `--aws-max-attempts`,
`--aws-max-backoff`, `--log-level`, `--report-file`, or `--config`.

### Action outputs

Every output is derived from the redacted report the CLI writes. The status,
count, and verification vocabularies are defined in
[results](results.md).

| Output | Description |
|--------|-------------|
| `status` | The run status from the report. |
| `cli-version` | The version reported by the executed binary. |
| `git-revision` | The exact Git revision reported by the CLI. |
| `create-count` | Planned or applied create count. |
| `update-count` | Planned or applied update count. |
| `restore-count` | Planned or applied restore count. |
| `scheduled-delete-count` | Planned or applied scheduled-deletion count. |
| `unchanged-count` | Unchanged resource count. |
| `conflict-count` | Conflict count. |
| `verification-status` | The verification status from the report. |
| `redacted-report-path` | Absolute path to the temporary redacted report. |

### Runner requirements

- **Runtime:** `node24`.
- **Platforms:** Linux and macOS only, on `X64` or `ARM64` runners. Any other
  runner operating system or architecture is rejected before installation.
- **Required runner environment:** `GITHUB_WORKSPACE`, `RUNNER_TEMP`,
  `RUNNER_OS`, `RUNNER_ARCH`, `GITHUB_SHA`, and `GITHUB_REPOSITORY`. `GITHUB_SHA`
  and `GITHUB_REPOSITORY` supply the defaults for `revision` and `repository-id`
  and are read even when those inputs are set explicitly.

## Cross-surface availability

- **CLI flags with no Action input:** `--repository`, `--max-encrypted-bytes`,
  `--aws-profile`, `--aws-max-attempts`, `--aws-max-backoff`, `--log-level`,
  `--log-format`, `--report-file`, `--config`, `--detailed-exit-code`.
- **Action inputs with no shared CLI flag:** `mode` (selects the subcommand),
  `fail-on-drift` (drives `--detailed-exit-code`), `cli-version`, and
  `github-token`.

## Output streams

- **stdout** carries the structured operational logs and the `version`,
  `--version`, and help text.
- **stderr** carries the Cobra usage line and configuration-validation
  diagnostics. Diagnostics state a reason only; rejected values are never
  echoed.
- **The report file** receives the machine-readable report, and only when
  `--report-file` is set.
- The bundled SOPS library's own logging is fully suppressed on both streams.

By default, resource identifiers in the logs and report are replaced with
`resource-NNNN` ordinals (for example `resource-0001`). `--show-resource-names`
includes real secret names and source paths instead.

## Report file mechanics

When `--report-file` (or `SOPS_AWS_SYNC_REPORT_FILE`) is set, the report is
written to a temporary file in the destination directory, flushed, and
atomically renamed into place with permission mode `0600`. The report is
written even when the run fails during setup, so it always reflects the final
status. A report write failure causes the command to exit `3`, overriding the
run's own outcome. The report schema and its guarantees are defined in
[results](results.md).

## Version output

The `version` subcommand and the `--version` flag both print a single line:

    sops-aws-sync <version> (<commit>) built <date>

The three fields are injected at build time. When a field is not injected it
falls back to `dev`, `none`, or `unknown` respectively.

## Deliberate exclusions

| Content | Owning document |
|---------|-----------------|
| Exit codes, run statuses, verification values, and the report schema | [Results and exit codes reference](results.md) |
| Source format, name mapping, the decision matrix, and conflict classes | [Reconciliation reference](reconciliation.md) |
| How to wire any option or input into a workflow | [How to deploy sops-aws-sync with GitHub Actions](../how-to/deploy-with-github-actions.md), [How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md) |
| Why precedence, redaction, and version pinning are designed this way | [About the security and trust model](../explanation/security-and-trust.md) |

## Related references

- [Results and exit codes reference](results.md) — exit codes, run statuses,
  verification values, and the report schema.
- [Reconciliation reference](reconciliation.md) — source selection, name
  mapping, the decision matrix, and the AWS operations issued.
- [How to deploy sops-aws-sync with GitHub Actions](../how-to/deploy-with-github-actions.md)
  — wiring these inputs into a workflow.
- [How to grant sops-aws-sync least-privilege AWS access](../how-to/grant-aws-access.md)
  — the AWS credentials and Region resolution these options depend on.

# Manual functional E2E acceptance test

Use a temporary private GitHub repository and a disposable AWS sandbox to test
the released GitHub Action exactly as an operator would use it. This is a
release-blocking acceptance test, not a repository test suite.

Execution status: **not run**

## Release decision

Pass only when every required scenario and cleanup check passes. Stop and treat
any of these as a release blocker:

- a plan or rejected input changes AWS;
- an unowned secret is adopted, changed, tagged, or deleted;
- a secret value, credential, private key, token, source path, or secret name
  appears in default logs or summaries;
- the Action executes an unverified or unexpected CLI version;
- a successful sync does not match direct AWS observations; or
- the temporary repository, token, or AWS sandbox cannot be removed.

A GitHub or Whizlabs outage is **inconclusive**, not a product failure. Preserve
the evidence, clean up, and repeat with fresh temporary resources.

## Current execution gates

As checked on 2026-07-20:

- `meigma/sops-aws-sync` is private and has no published release.
- `action.yml` defaults to CLI `0.1.1`, but that metadata is not a release.
- private Action access is `none`, so another private organization repository
  cannot currently load it.
- the current GitHub login cannot inspect the organization-wide Actions policy.

Do not call a dry-run artifact a full E2E pass. Before execution, require:

1. A published immutable release or prerelease whose Action metadata, tag,
   binaries, checksums, and attestations all carry the same exact version.
2. The full commit SHA behind that release.
3. Explicit approval to make the private producer Action accessible to private
   repositories in `meigma` for the test, using
   [GitHub's private Action access setting][private-action-access], followed by
   restoration of the original setting.
4. A temporary, read-only producer token restricted to
   `meigma/sops-aws-sync` contents and attestations. Do not upload the broad
   token returned by `gh auth token`.
5. Confirmation from an organization owner that the organization Actions
   policy permits the pinned producer Action.
6. A GitHub credential able to delete the temporary organization repository,
   or a confirmed UI deletion path.
7. Exclusive use of the Whizlabs account for the duration of the run.

If the stable release must not be published before acceptance, publish an
explicitly authorized prerelease candidate and align the Action version first.

## Boundaries

- Use only `git`, `gh`, `whzbox`, `aws`, `sops`, `age-keygen`, `jq`, and
  normal GitHub UI/Actions features.
- Use harmless, recognizable canary values. Never use a real application
  secret.
- Add no test harness, custom executable, pull-request workflow, reusable
  privileged workflow, or malicious payload.
- Keep `show-resource-names` false.
- Trigger privileged work only with `workflow_dispatch` from the default
  branch.
- Use one non-cancelling concurrency group for the test ownership scope.
- Do not enable shell tracing or record the credential-handoff terminal.

## Test record

Fill this in before the first dispatch.

| Item | Value |
| --- | --- |
| Operator and start time | |
| Release tag and version | |
| Full producer commit SHA | |
| Release URL | |
| Temporary consumer repository | |
| Consumer default branch | |
| AWS account, Region, and expiry | |
| Unique Secrets Manager prefix | |
| Original producer Action access | |
| Scenario run URLs | |

## 1. Prepare isolated temporary resources

### Verify tools and release

Set the candidate version first:

```sh
export E2E_VERSION="<version-without-v>"
export E2E_TAG="v$E2E_VERSION"
```

Record command output without secrets:

```sh
gh auth status
gh repo view meigma/sops-aws-sync --json nameWithOwner,visibility,viewerPermission
gh release view "$E2E_TAG" -R meigma/sops-aws-sync --json tagName,isDraft,isPrerelease,assets,url
gh api repos/meigma/sops-aws-sync/actions/permissions/access
command -v whzbox
whzbox version
shasum -a 256 "$(command -v whzbox)"
aws --version
sops --version
age-keygen -version
```

Resolve and record the exact release commit:

```sh
export E2E_ACTION_SHA="$(gh api "repos/meigma/sops-aws-sync/commits/$E2E_TAG" --jq .sha)"
test "$(printf '%s' "$E2E_ACTION_SHA" | wc -c | tr -d ' ')" = 40
export E2E_PAIRED_VERSION="$(gh api -H 'Accept: application/vnd.github.raw+json' "repos/meigma/sops-aws-sync/contents/action.yml?ref=$E2E_ACTION_SHA" | sed -n 's/^    default: \([^ ]*\) # x-release-please-version$/\1/p')"
test "$E2E_PAIRED_VERSION" = "$E2E_VERSION"
test "$(gh release view "$E2E_TAG" -R meigma/sops-aws-sync --json isDraft --jq .isDraft)" = false
gh release view "$E2E_TAG" -R meigma/sops-aws-sync --json assets --jq '.assets[].name' | rg -Fx checksums.txt
gh release view "$E2E_TAG" -R meigma/sops-aws-sync --json assets --jq '.assets[].name' | rg -Fx "sops-aws-sync_${E2E_VERSION}_linux_amd64"
```

With explicit approval, record the original private Action access and set it to
`organization` for this run. Restore the recorded value during cleanup.

```sh
export E2E_ORIGINAL_ACTION_ACCESS="$(gh api repos/meigma/sops-aws-sync/actions/permissions/access --jq .access_level)"
gh api --method PUT repos/meigma/sops-aws-sync/actions/permissions/access -f access_level=organization
```

### Create an isolated whzbox state

`whzbox` stores sandbox credentials locally. Treat its state directory as a
secret. `destroy` targets the Whizlabs account's active sandbox without a local
sandbox ID, which is why the account must not be used concurrently.

```sh
unset WHZBOX_JSON WHZBOX_YES WHZBOX_WHIZLABS_BASE_URL WHZBOX_WHIZLABS_PLAY_URL
export E2E_WHZ_STATE="$(mktemp -d)"
chmod 700 "$E2E_WHZ_STATE"
export WHZBOX_STATE_DIR="$E2E_WHZ_STATE"

whzbox login
whzbox create aws --duration 4h >/dev/null
whzbox list
whzbox exec aws -- aws sts get-caller-identity
```

From this point onward, cleanup is mandatory even if setup fails. Avoid
`create --json`, `list --json`, environment dumps, and shell tracing because
they can expose cached credentials.

### Create the private consumer repository

```sh
export E2E_RUN_ID="$(date -u +%Y%m%d%H%M%S)"
export E2E_REPO="meigma/sops-aws-sync-e2e-$E2E_RUN_ID"
export E2E_ROOT="$(mktemp -d)"

cd "$E2E_ROOT"
gh repo create "$E2E_REPO" --private --add-readme --clone
cd "$(basename "$E2E_REPO")"

export E2E_BRANCH="$(gh repo view "$E2E_REPO" --json defaultBranchRef --jq .defaultBranchRef.name)"
export E2E_PREFIX="/sops-aws-sync-e2e/$E2E_RUN_ID"
export E2E_REGION="$(whzbox exec aws -s 'printf "%s" "$AWS_REGION"')"
export E2E_APP_NAME="$E2E_PREFIX/app"
export E2E_WORKER_NAME="$E2E_PREFIX/nested/worker"
export E2E_API_NAME="$E2E_PREFIX/api"
export E2E_COLLISION_NAME="$E2E_PREFIX/collision"
export E2E_OUTSIDE_NAME="/sops-aws-sync-e2e-outside/$E2E_RUN_ID"

gh variable set AWS_REGION -R "$E2E_REPO" --body "$E2E_REGION"
gh variable set SECRET_PREFIX -R "$E2E_REPO" --body "$E2E_PREFIX"
```

Pass the sandbox credentials directly from `whzbox` to GitHub's encrypted
secret input. Do not print them or place them in command arguments:

This temporary static-key handoff is an explicit sandbox-only exception to the
normal OIDC recommendation. Never reuse it for a durable repository or AWS
account.

```sh
export E2E_REPO
whzbox exec aws -s '
  printf "%s=%s\n" \
    AWS_ACCESS_KEY_ID "$AWS_ACCESS_KEY_ID" \
    AWS_SECRET_ACCESS_KEY "$AWS_SECRET_ACCESS_KEY" |
  gh secret set --repo "$E2E_REPO" --app actions -f -
'
gh secret list -R "$E2E_REPO"
```

Generate a temporary age identity, store only its private identity as a GitHub
secret, and retain the `0600` local file until cleanup:

```sh
umask 077
export E2E_AGE_KEY="$E2E_ROOT/age-key.txt"
age-keygen -o "$E2E_AGE_KEY"
export E2E_AGE_RECIPIENT="$(sed -n 's/^# public key: //p' "$E2E_AGE_KEY")"
sed -n '/^AGE-SECRET-KEY-/p' "$E2E_AGE_KEY" | gh secret set SOPS_AGE_KEY -R "$E2E_REPO"
```

Create and commit three ordinary SOPS files with these harmless plaintext
canaries:

| Committed path | Mapped AWS name | Initial value marker |
| --- | --- | --- |
| `secrets/app.sops.json` | `<prefix>/app` | `E2E_APP_V1` |
| `secrets/nested/worker.sops.json` | `<prefix>/nested/worker` | `E2E_WORKER_V1` |
| `secrets/collision.sops.json` | `<prefix>/collision` | `E2E_COLLISION_DESIRED` |

Use `sops --encrypt --age "$E2E_AGE_RECIPIENT"`. Commit only the encrypted
`*.sops.json` files; remove any local plaintext files immediately.

Use this ordinary user workflow for each document, changing the marker and
destination as needed:

```sh
mkdir -p secrets/nested
export E2E_PLAINTEXT="$E2E_ROOT/plaintext.json"
printf '%s\n' '{"marker":"E2E_APP_V1"}' >"$E2E_PLAINTEXT"
sops --encrypt --age "$E2E_AGE_RECIPIENT" --input-type json --output-type json "$E2E_PLAINTEXT" >secrets/app.sops.json
rm -f "$E2E_PLAINTEXT"
```

Create all three encrypted documents, add the workflow below, then commit and
push them together. Record every later state change with the same normal
`git add`, `git commit`, and `git push` flow.

### Add the operator workflow

Create `.github/workflows/e2e.yml`. Replace the producer placeholder with
`$E2E_ACTION_SHA` before committing.

```yaml
name: Manual sops-aws-sync E2E

on:
  workflow_dispatch:
    inputs:
      mode:
        type: choice
        options: [plan, sync]
        default: plan
      fail_on_drift:
        type: boolean
        default: false
      allow_empty:
        type: boolean
        default: false
      revision:
        type: string
        required: false

permissions:
  contents: read
  attestations: read

concurrency:
  group: sops-aws-sync-e2e-${{ github.repository_id }}
  cancel-in-progress: false

jobs:
  reconcile:
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0
        with:
          fetch-depth: 0
          persist-credentials: false

      - uses: aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ vars.AWS_REGION }}
          mask-aws-account-id: true

      - name: Reconcile committed SOPS documents
        uses: meigma/sops-aws-sync@<FULL_RELEASE_COMMIT_SHA>
        env:
          SOPS_AGE_KEY: ${{ secrets.SOPS_AGE_KEY }}
        with:
          mode: ${{ inputs.mode }}
          fail-on-drift: ${{ inputs.fail_on_drift }}
          allow-empty: ${{ inputs.allow_empty }}
          revision: ${{ inputs.revision }}
          source-root: secrets
          secret-prefix: ${{ vars.SECRET_PREFIX }}
          aws-region: ${{ vars.AWS_REGION }}
          recovery-window-days: 7
          github-token: ${{ secrets.PRODUCER_READ_TOKEN }}
```

Commit and push the workflow and encrypted files. Confirm that the repository is
private and the workflow has only a manual trigger.

### Run and inspect a dispatch

Bind every dispatch to its exact run and consumer commit before judging it. Do
this once per dispatch, including each back-to-back run:

```sh
export E2E_EXPECTED_HEAD="$(git rev-parse HEAD)"
export E2E_BEFORE_RUN="$(gh run list -R "$E2E_REPO" --workflow e2e.yml --commit "$E2E_EXPECTED_HEAD" --event workflow_dispatch --limit 1 --json databaseId --jq '.[0].databaseId // 0')"
gh workflow run e2e.yml -R "$E2E_REPO" --ref "$E2E_BRANCH" -f mode=plan -f fail_on_drift=false -f allow_empty=false

E2E_POLL_ATTEMPT=0
while test "$E2E_POLL_ATTEMPT" -lt 30; do
  E2E_GH_RUN="$(gh run list -R "$E2E_REPO" --workflow e2e.yml --commit "$E2E_EXPECTED_HEAD" --event workflow_dispatch --limit 1 --json databaseId --jq '.[0].databaseId // 0')"
  test "$E2E_GH_RUN" != "$E2E_BEFORE_RUN" && test "$E2E_GH_RUN" != 0 && break
  E2E_POLL_ATTEMPT=$((E2E_POLL_ATTEMPT + 1))
  sleep 2
done
export E2E_GH_RUN
test "$E2E_GH_RUN" != "$E2E_BEFORE_RUN" && test "$E2E_GH_RUN" != 0
test "$(gh run view "$E2E_GH_RUN" -R "$E2E_REPO" --json headSha --jq .headSha)" = "$E2E_EXPECTED_HEAD"
gh run watch "$E2E_GH_RUN" -R "$E2E_REPO" --compact --exit-status
gh run view "$E2E_GH_RUN" -R "$E2E_REPO" --web
```

For an expected failure, a nonzero `gh run watch --exit-status` is the expected
shell result. The Action-generated summary is the source for status, exact CLI
version, Git revision, operation counts, conflicts, and verification.

## 2. Required scenarios

Each scenario builds on the state left by the previous one.

### Scenario A — Release trust and non-mutating plan

1. Set `PRODUCER_READ_TOKEN` to the harmless value
   `deliberately-invalid` and dispatch `plan`.
2. Confirm the Action fails while downloading or verifying the exact release.
3. Set the approved short-lived producer-read token through
   `gh secret set PRODUCER_READ_TOKEN -R "$E2E_REPO"` without putting it on the
   command line.
4. Dispatch `plan` again against the three-file commit.

Pass:

- the invalid-token run fails before the CLI or AWS reconciliation starts;
- the valid run verifies the release, reports the expected CLI version and
  consumer commit, `status=drift`, `create=3`, all other operation/conflict
  counts zero, and `verification=not-run`; and
- no secret exists under the test prefix after either plan.

Fail if installation falls back to another binary, the reported version or
revision differs, either plan mutates AWS, or token/release internals leak.

### Scenario B — Ownership guard, create, and repeat safety

1. Create an untagged secret named `$E2E_COLLISION_NAME` with a harmless foreign
   value. Also create the outside-prefix sentinel `$E2E_OUTSIDE_NAME`.
2. Record both sentinels' value hashes, tags, and lifecycle state.
3. Dispatch `sync` while `collision.sops.json` is still desired.
4. Remove `collision.sops.json`, commit, and push. Leave the foreign collision
   secret in AWS as a same-prefix sentinel.
5. Dispatch `sync` once, then dispatch two more `sync` runs back-to-back.

Create the sentinels with normal AWS CLI calls; their values are public test
markers, not credentials:

```sh
whzbox exec aws -- aws secretsmanager create-secret --region "$E2E_REGION" --name "$E2E_COLLISION_NAME" --secret-string '{"marker":"E2E_FOREIGN_COLLISION"}' >/dev/null
whzbox exec aws -- aws secretsmanager create-secret --region "$E2E_REGION" --name "$E2E_OUTSIDE_NAME" --secret-string '{"marker":"E2E_FOREIGN_OUTSIDE"}' >/dev/null
```

Pass:

- the collision run reports `status=conflict` and changes neither the collision
  secret nor the two other desired names;
- the first clean sync reports `status=converged`, `create=2`, and
  `verification=converged`;
- the repeat runs are never both `in_progress` at the same time (one is queued
  if the first is still active), and both report `unchanged=2`; and
- both foreign sentinels retain the same value hashes, tags, and lifecycle
  state.

Fail if the foreign secret is adopted or if any desired secret is created while
the conflicting plan is rejected.

Record this two-file commit as `COMMIT_A`.

### Scenario C — Exact commit and the complete lifecycle

1. Commit `COMMIT_B`: change app to `E2E_APP_V2`, remove worker, and add
   `secrets/api.sops.json` with `E2E_API_V1`.
2. Dispatch from `COMMIT_B` with `revision=COMMIT_A` and `mode=plan`.
3. Dispatch a normal `COMMIT_B` plan with `fail_on_drift=true`, then sync it.
4. Commit `COMMIT_C` by reintroducing worker as `E2E_WORKER_V2` and sync.
5. Change app directly through the AWS CLI to `E2E_EXTERNAL_DRIFT`, then sync
   `COMMIT_C` again.
6. Run one final no-op sync.

Create the external drift without printing the AWS response:

```sh
whzbox exec aws -- aws secretsmanager put-secret-value --region "$E2E_REGION" --secret-id "$E2E_APP_NAME" --secret-string '{"marker":"E2E_EXTERNAL_DRIFT"}' >/dev/null
```

Pass:

- the old-revision plan reports `COMMIT_A`, `status=converged`, and
  `unchanged=2`;
- the drift plan fails intentionally, reports one create, one update, and one
  scheduled deletion, and does not mutate;
- the `COMMIT_B` sync applies those same three operations, verifies app/API
  values directly, and shows a worker deletion date about seven days out;
- `COMMIT_C` reports `restore=1`, `update=1`, `unchanged=2`, directly verifies
  worker V2, and converges;
- the next sync repairs direct AWS drift; and
- the final sync reports three unchanged secrets.

Fail if the current commit overrides the selected old revision, deletion is
immediate, restore or external-drift repair fails, or reported convergence
disagrees with direct `GetSecretValue`/`DescribeSecret` checks.

### Scenario D — Bad committed source is all-or-nothing

1. Commit a legitimate app change to `E2E_APP_V3` together with an accidental
   plaintext `secrets/00-accidental.sops.json` containing the harmless marker
   `E2E_PLAINTEXT_MUST_NOT_LEAK`.
2. Dispatch `sync`.
3. Download the run log locally and scan it before deleting it.
4. Remove the accidental file, commit the repaired desired state, and sync
   again.

Pass:

- the first run reports `status=invalid` and performs no AWS mutation, including
  the otherwise valid app update;
- the failure is short and safe rather than a raw SOPS/provider error;
- the unmasked plaintext marker, decrypted values, source paths, and secret
  names do not appear in logs or summary; and
- the repaired committed state reports `update=1`, `unchanged=2`, and
  converges.

Fail on any partial mutation or sensitive/default-hidden output.

### Scenario E — Empty-state gate and scope isolation

1. Commit removal of all three managed `*.sops.json` files.
2. Dispatch `sync` with `allow_empty=false`.
3. Dispatch `plan` with `allow_empty=true` and `fail_on_drift=true`.
4. Dispatch `sync` with `allow_empty=true`.
5. Recheck both foreign sentinels.

Pass:

- the first run reports `status=invalid` and three proposed scheduled
  deletions, but schedules nothing;
- the authorized plan reports `status=drift`, three scheduled deletions, and
  `verification=not-run`, but mutates nothing;
- the authorized sync schedules exactly the three owned secrets with a
  seven-day recovery window, reports convergence, and never force-deletes
  them; and
- the same-prefix foreign sentinel and outside-prefix sentinel are byte-for-byte
  and tag-for-tag unchanged.

Fail if an empty desired set acts without approval, any deletion is immediate,
or any foreign resource changes.

## 3. Cross-cutting evidence

For every run, record:

- run URL and conclusion;
- consumer head SHA and selected Git revision;
- exact Action SHA and CLI version;
- Action summary status, counts, conflicts, and verification;
- direct AWS value comparisons without printing `SecretString`; and
- `DeletedDate` plus reserved ownership tags when lifecycle state matters.

Use hashes for value evidence. Record the expected hash before encryption, then
compare it with a direct AWS read without printing or saving `SecretString`:

```sh
printf '%s\n' '{"marker":"E2E_APP_V1"}' | shasum -a 256
whzbox exec aws -- aws secretsmanager get-secret-value --region "$E2E_REGION" --secret-id "$E2E_APP_NAME" --query SecretString --output text | shasum -a 256
whzbox exec aws -- aws secretsmanager describe-secret --region "$E2E_REGION" --secret-id "$E2E_APP_NAME" --query '{DeletedDate:DeletedDate,Tags:Tags}'
```

Never capture raw AWS CLI output containing `SecretString` in the test record.

Download raw logs only into `$E2E_ROOT`. Search them for the known plaintext
markers, full source paths, and full secret names:

```sh
export E2E_LOG="$E2E_ROOT/run-$E2E_GH_RUN.log"
gh run view "$E2E_GH_RUN" -R "$E2E_REPO" --log >"$E2E_LOG"
rg -n -F -e E2E_PLAINTEXT_MUST_NOT_LEAK -e secrets/00-accidental.sops.json -e "$E2E_APP_NAME" "$E2E_LOG"
```

The `rg` command must find nothing. GitHub automatically masks values stored as
Actions secrets, so a credential scan cannot prove application-level
redaction. Use this only as defense in depth, and inspect unexplained `***`
output in the reconcile step:

```sh
whzbox exec aws -s '
  ! rg -Fq -- "$AWS_ACCESS_KEY_ID" "$E2E_LOG" &&
  ! rg -Fq -- "$AWS_SECRET_ACCESS_KEY" "$E2E_LOG"
'
sed -n '/^AGE-SECRET-KEY-/p' "$E2E_AGE_KEY" |
  while IFS= read -r key; do ! rg -Fq -- "$key" "$E2E_LOG"; done
```

Do not preserve raw logs after recording a sanitized result.

Do not force a partial-failure or cancellation window with injected faults or a
custom test harness. If an ordinary sync remains active long enough, cancelling
it once and rerunning the same commit is useful additional evidence; otherwise
record interruption recovery as **not exercised**, not passed.

## 4. Cleanup and proof

Cleanup is part of the acceptance result. On an emergency stop, cancel workflows
and destroy the AWS sandbox first.

1. Cancel any queued or running consumer workflows.
2. Revoke the temporary producer-read token.
3. Restore the producer's original private Action access setting:
   `gh api --method PUT repos/meigma/sops-aws-sync/actions/permissions/access -f "access_level=$E2E_ORIGINAL_ACTION_ACCESS"`.
4. Delete the temporary GitHub repository with
   `gh repo delete "$E2E_REPO" --yes` and verify `gh repo view "$E2E_REPO"`
   returns not found.
5. Destroy the AWS sandbox with `whzbox destroy --yes`.
6. Run `whzbox list` and require `(no sandboxes cached)`.
7. Run `whzbox destroy --yes` once more. The expected provider result is
   “no active sandbox”; this is the strongest available upstream cleanup check.
8. Run `whzbox logout`. Require `$WHZBOX_STATE_DIR/state.json` to be absent.
9. Remove the temporary clone, age key, raw logs, and empty whzbox state
   directory. Unset all `E2E_*` and `WHZBOX_STATE_DIR` variables.

If repository deletion fails, disable Actions and delete all Actions secrets
first, confirm producer Action access is restored, destroy the sandbox, and
finish GitHub cleanup through the UI.

## Final result

Record one of:

- **PASS** — every required scenario and cleanup check passed;
- **FAIL** — product behavior violated a pass criterion; release is blocked; or
- **INCONCLUSIVE** — infrastructure prevented a valid observation and all
  temporary resources were still cleaned up.

List exact failed scenario steps and run URLs. Do not soften a leak, foreign
resource mutation, provenance bypass, false convergence, or cleanup failure
into an inconclusive result.

[private-action-access]: https://docs.github.com/actions/creating-actions/sharing-actions-and-workflows-with-your-organization

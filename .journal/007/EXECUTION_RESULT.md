# Manual E2E acceptance result

Date: 2026-07-20 PDT / 2026-07-21 UTC
Decision: **FAIL — release blocker; do not release**

The live acceptance run stopped at its required release gate. No product
scenario was allowed to run without a published, exact-version CLI release and
GitHub provenance attestations.

## What passed

- Producer `master`, `origin/master`, and the candidate source were identical at
  `b67210d287faa642f7b4e1d7e73da438b5253abb`.
- CI passed on that SHA:
  <https://github.com/meigma/sops-aws-sync/actions/runs/29793089486>.
- Release metadata agreed on version `0.1.1` in
  `.release-please-manifest.json`, `action/package.json`, and `action.yml`.
- The exact-SHA Release Dry Run passed in 10m19s, including Action bundle
  verification, GoReleaser, asset staging, and the binary smoke test:
  <https://github.com/meigma/sops-aws-sync/actions/runs/29795664004>.
- Organization-only private Action access could be enabled and later restored.
- A disposable AWS sandbox could be created and verified with `whzbox`.

## Release blocker

An existing draft prerelease for `v0.1.1` and an annotated tag resolving to the
exact producer SHA triggered the real Release workflow:
<https://github.com/meigma/sops-aws-sync/actions/runs/29796150738>.

`Resolve Release` attempted this check 30 times over five minutes:

```sh
gh release view "$RELEASE_TAG" --repo "$GITHUB_REPOSITORY" --json isDraft
```

Every attempt failed, ending with `Draft release was not found or is not a
draft`. At the same time, an administrator could retrieve the release and
confirm `isDraft=true`, `isPrerelease=true`, the exact target SHA, and an empty
asset list.

The handoff is internally incompatible: GitHub's `Get a release by tag name`
endpoint returns a **published** release, while this workflow requires that
same result to be a draft. The asset-build and attestation jobs were therefore
skipped.

Minimum correction to validate in a normal implementation change:

1. Give the resolver push-level release visibility, normally
   `contents: write`, because GitHub exposes draft release listings only to
   identities with push access.
2. Find the draft through the release-list endpoint and match both
   `tag_name == $RELEASE_TAG` and `draft == true`; do not use the published-only
   by-tag endpoint.
3. Preserve the current bounded wait, then rerun the real release workflow and
   verify its assets and attestations before publishing the prerelease.

No workflow or product code was changed during this acceptance run.

## Scenarios A–E

**Not run.** Running them would have required bypassing the plan's exact-release
and provenance gate. This is a release-blocking result, not a partial pass.

The narrow producer-read token also could not be created unattended: GitHub's
fine-grained token UI reached sudo mode and required a user-held security key,
GitHub Mobile approval, authenticator code, or password. The broad token from
`gh auth token` was not uploaded or used as a fallback.

## Safety event

During the first `whzbox create`, the operator omitted output suppression and
temporary sandbox credentials appeared in the operator transcript. That
sandbox was immediately treated as compromised and destroyed before any
credential was uploaded or any AWS resource was created. A new sandbox was
then created with output suppressed and verified normally.

This was an execution error, not a product-log leak. The test plan already
requires `whzbox create ... >/dev/null`; future runs must use that command
verbatim.

## Cleanup verification

- First exposed AWS sandbox: destroyed; credentials invalidated.
- Replacement AWS sandbox: destroyed without use.
- Isolated Whizlabs state directory: deleted; not recoverable locally.
- Draft `v0.1.1` release: deleted.
- Remote and local `v0.1.1` test tag: deleted.
- Producer private Action access: restored to `none`.
- Fine-grained producer token: never created.
- Temporary consumer repository: never created.
- Consumer Actions secrets and AWS Secrets Manager resources: never created.
- Producer worktree: clean and unchanged.

The failed GitHub Actions run and exact-SHA rehearsal remain as non-secret audit
evidence.

## Resume attempt on 2026-07-20

PR #12 fixed the draft-release handoff without weakening the release gates. It
passed hosted checks on exact head
`e3862c88c10824797af7b56329f565cd3684657a` and squash-merged as
`0894dfe568f9a1cb6df616d2cfb21260a4b7061f`.

The real `v0.1.1` release run then confirmed the fix:
<https://github.com/meigma/sops-aws-sync/actions/runs/29796898446>.

- draft resolution passed in two seconds;
- all four binaries and four SBOMs were built, validated, and uploaded with
  `checksums.txt`;
- the host binary smoke test passed; and
- the isolated attestation job downloaded the exact checksum artifact.

GitHub rejected the final provenance write because artifact attestations are
not available for this private repository on the `meigma` organization's
current plan. GitHub instructed the operator to upgrade the billing plan or
make the repository public. The release cannot be published and the Action
cannot verify its binary without that attestation.

Current retained state is intentionally non-public and credential-free:

- `v0.1.1` is an exact-SHA draft prerelease with nine assets;
- the exact annotated tag remains at the merged fix SHA;
- producer private Action access is `none`; and
- no consumer repository, narrow producer token, or AWS sandbox exists.

Scenarios A–E remain blocked. The provenance requirement must not be bypassed.

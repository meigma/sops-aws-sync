# Manual E2E acceptance result

Date: 2026-07-20 PDT / 2026-07-21 UTC
Decision: **PASS — no release blocker found after remediation**

The released Action was exercised as an ordinary user from a temporary private
consumer repository against a disposable AWS account. The run covered release
trust, planning, ownership conflicts, concurrency, exact revisions, create,
update, restore, scheduled deletion, drift repair, invalid plaintext input,
log redaction, empty desired state, scope boundaries, and cleanup.

## Candidate and release repair

- Producer: public `meigma/sops-aws-sync`.
- Release: immutable prerelease `v0.1.1` at
  <https://github.com/meigma/sops-aws-sync/releases/tag/v0.1.1>.
- Release commit: `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`.
- Release run:
  <https://github.com/meigma/sops-aws-sync/actions/runs/29796898446>.
- `gh release verify v0.1.1` verified all four binaries, four SBOMs, and
  `checksums.txt` against GitHub provenance.

The first live release attempt exposed a real blocker: the release resolver
used a published-only by-tag endpoint while waiting for a draft. PR #12 granted
the resolver `contents: write` and exact-matched the draft tag through the
release-list endpoint. All hosted checks passed on exact head
`e3862c88c10824797af7b56329f565cd3684657a`; it squash-merged as the release
commit above. After the repository became public, the retained attestation job
reran successfully and the release was published immutably.

## Scenario results

### A — Release trust and non-mutating plan: PASS

- Invalid credential run failed during release resolution, before CLI or AWS
  reconciliation:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797617347>.
- Public-producer plan verified CLI `0.1.1`, bound the exact consumer revision,
  reported `drift` with `create=3`, and left the AWS prefix empty:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797733977>.

### B — Ownership, create, and repeat safety: PASS

- An untagged same-prefix collision caused `status=conflict`; no desired secret
  was created and both foreign sentinels were unchanged:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797782242>.
- Removing the collision converged with two creates:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797840969>.
- Two back-to-back dispatches queued instead of cancelling or overlapping, then
  both converged at the same exact revision with `unchanged=2`:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797876924>
  and
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29797877914>.

### C — Exact revision and full lifecycle: PASS

- A plan dispatched from a newer head but pinned to the older commit reported
  that older SHA and `unchanged=2`:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798045765>.
- A current-head plan with `fail-on-drift=true` failed intentionally with
  `create=1`, `update=1`, and `scheduled-delete=1`, without mutation:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798074890>.
- Sync applied those operations and direct AWS observations matched the
  committed JSON semantically; the removed worker had a seven-day recovery
  date:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798094946>.
- Reintroducing the worker restored it and updated its value:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798186338>.
- A direct out-of-band AWS edit was repaired on the next sync; the following
  sync reported all three secrets unchanged:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798217416>
  and
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798234715>.

### D — Invalid plaintext and redaction: PASS

- A plaintext `*.sops.json` user mistake produced `status=invalid` before any
  AWS mutation:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798291914>.
- The raw hosted log contained none of the plaintext canary, offending source
  path, full secret names, age private key, or AWS sandbox credentials.
- Removing only the invalid file allowed the pending encrypted update to
  converge with `update=1` and `unchanged=2`:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798324561>.

### E — Empty source and scope boundary: PASS

- Deleting the final tracked source file also removed the source directory from
  Git. Both the default sync and an `allow-empty=true` plan rejected that
  missing source root without mutation:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798364527>
  and
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798403496>.
- After adding `secrets/.gitkeep`, an authorized plan reported
  `scheduled-delete=3` and changed nothing:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798450124>.
- Authorized sync scheduled exactly three recovery-window deletions and
  verified convergence:
  <https://github.com/meigma/sops-aws-sync-e2e-20260721030410/actions/runs/29798494616>.
- The unowned same-prefix collision and out-of-prefix sentinel retained their
  original value hashes, null tags, and null deletion dates throughout.

Observation: an intentionally empty committed source needs a tracked placeholder
such as `.gitkeep`; otherwise it is a missing source root, not an empty source.
That fail-closed behavior is safe but worth documenting for operators.

## Safety assessment

- Plans and rejected inputs did not mutate AWS.
- Unowned and out-of-scope secrets were never adopted, tagged, changed, or
  scheduled for deletion.
- Deletions always used the seven-day recovery window; force deletion was not
  available.
- The Action used the expected CLI version and exact consumer revisions.
- Default hosted logs masked GitHub/AWS credentials and did not disclose tested
  secret values, private keys, full source paths, or full secret names.
- The Action workflow was manual-only, pinned third-party Actions by SHA, used
  read-only GitHub permissions, and serialized runs with non-cancelling
  concurrency.

One operator-side setup event occurred before the successful run: an initial
`whzbox create` was invoked without output suppression and printed disposable
sandbox credentials. That sandbox was immediately destroyed before use and a
fresh isolated sandbox was created with output suppressed. This was not a
product-log disclosure.

## Cleanup verification

- Temporary private consumer repository
  `meigma/sops-aws-sync-e2e-20260721030410`: deleted by the user and confirmed
  absent through GitHub.
- Consumer secrets, variables, commits, and workflow runs: removed with the
  repository.
- AWS sandbox account `705991249149`: destroyed; `whzbox list` returned no
  cached sandboxes and a second destroy failed because no sandbox remained.
- Isolated Whizlabs login state: logged out and moved to Trash.
- Local consumer clone, plaintext scratch files, and age private key: moved to
  Trash.
- Temporary producer token: not needed after the producer became public and
  therefore never retained.
- Producer: public, clean, and synchronized at
  `0894dfe568f9a1cb6df616d2cfb21260a4b7061f`.
- Release `v0.1.1`: intentionally retained as the verified immutable
  prerelease candidate.

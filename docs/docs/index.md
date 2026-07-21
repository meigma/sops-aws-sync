---
title: sops-aws-sync operator documentation
---

# sops-aws-sync operator documentation

sops-aws-sync is a Go command-line tool paired with a supply-chain-verified
Node 24 GitHub Action. It reconciles the SOPS-encrypted `.sops.json`,
`.sops.yaml`, and `.sops.yml` documents committed at one exact Git commit into
an explicitly owned namespace in AWS Secrets Manager, issuing create, update,
restore, and scheduled-delete operations so that AWS matches what Git holds.
`plan` is read-only and reports what would change; `sync` mutates and then
always verifies that the scope converged.

The one sentence that governs everything else: the committed Git revision is
the desired state — you change AWS by committing and pushing those documents,
never by editing secrets directly. [About the reconciliation
model](explanation/reconciliation-model.md) explains why.

## Read before you run `sync`

Five invariants an operator must hold. Each is a pointer; the linked page owns
the full explanation.

- **Commit is the source of truth.** `sync` acts on the committed revision, not
  your working tree, index, or unpushed changes. →
  [reconciliation model](explanation/reconciliation-model.md)
- **Scope inputs are an identity, not a setting.** Changing `repository-id`,
  `source-root`, or `secret-prefix` mints a new scope that owns nothing prior
  and silently abandons the old secrets. →
  [ownership and scope](explanation/ownership-and-scope.md)
- **Emptying the source tree does not mass-delete.** A desired state with no
  files and pending deletions is refused unless you explicitly authorize it. →
  [decommission a scope](how-to/decommission-a-scope.md)
- **One writer per scope.** AWS offers no cross-secret transaction, so
  correctness depends on exactly one active `sync` per scope. →
  [consistency and recovery](explanation/consistency-and-recovery.md)
- **Not everything in a source document is secret.** SOPS encrypts values,
  not keys, paths, or metadata; those are plaintext in Git. →
  [security and trust](explanation/security-and-trust.md)

## Find your way

- **Understand how it works** — the four explanations are the backbone of this
  set; read them in order:
  [the reconciliation model](explanation/reconciliation-model.md),
  [ownership and scope](explanation/ownership-and-scope.md),
  [consistency and recovery](explanation/consistency-and-recovery.md), and
  [security and trust](explanation/security-and-trust.md).
- **Learn by doing** —
  [your first reconciliation](tutorial/first-reconciliation.md) against a
  throwaway scope.
- **Deploy in CI** —
  [deploy with GitHub Actions](how-to/deploy-with-github-actions.md), then
  [grant AWS access](how-to/grant-aws-access.md).
- **Run the CLI directly** —
  [verify a release binary](how-to/verify-a-release-binary.md), then the
  [tutorial](tutorial/first-reconciliation.md).
- **Look something up** — the three references:
  [configuration](reference/configuration.md),
  [reconciliation](reference/reconciliation.md), and
  [results and exit codes](reference/results.md).
- **A run failed** —
  [diagnose and recover](how-to/diagnose-and-recover.md).
- **Remove secrets** —
  [decommission or empty a scope](how-to/decommission-a-scope.md).

## What this tool is not

- **Not a secret editor.** It writes only what Git holds and reconciles AWS
  toward it. → [reconciliation model](explanation/reconciliation-model.md)
- **Not an AWS authenticator.** Bring credentials through the standard AWS SDK
  chain; the tool accepts no credential inputs. →
  [grant AWS access](how-to/grant-aws-access.md)
- **Not a manager of special secrets.** It refuses secrets that are under
  rotation, replicated, or AWS service-owned rather than touching them. →
  [ownership and scope](explanation/ownership-and-scope.md)
- **No force delete.** Every deletion is a scheduled deletion inside a recovery
  window. → [reconciliation model](explanation/reconciliation-model.md)
- **Three SOPS encodings, one stored form.** It reads `.sops.json`,
  `.sops.yaml`, and `.sops.yml` documents, stores every one as canonical JSON,
  and ignores every other file. →
  [reconciliation reference](reference/reconciliation.md)
- **Linux and macOS only**, on amd64 and arm64. →
  [configuration reference](reference/configuration.md)

## Elsewhere

- The repository
  [README](https://github.com/meigma/sops-aws-sync/blob/master/README.md) has
  the copy-paste Action snippet and build-from-source instructions.
- [SECURITY.md](https://github.com/meigma/sops-aws-sync/blob/master/SECURITY.md)
  covers private vulnerability reporting.

These pages serve operators who deploy and run the tool. Contributor and
release-pipeline internals are out of scope.

---
title: sops-aws-sync Docs
---

# sops-aws-sync

`sops-aws-sync` reconciles committed SOPS-encrypted JSON documents with AWS Secrets Manager through a safe, deterministic Go CLI.

The current Phase 2 implementation supports direct desired-name planning and create, update, no-op, and post-apply verification. Run `sops-aws-sync plan --help` or `sops-aws-sync sync --help` for the complete typed configuration surface.

---
id: 009
title: YAML-encoded SOPS support
date: 2026-07-21
status: complete
repos_touched: [sops-aws-sync]
related_sessions: [001, 002, 003, 004, 005, 006, 007]
---

## Goal
Add straightforward support for YAML-encoded SOPS files while preserving the existing canonical JSON and AWS reconciliation behavior.

## Outcome
Goal met. PR #13 added strict `.sops.yaml` and `.sops.yml` support, passed the complete local and hosted gates, and was squash-merged as `f602168`.

## Key Decisions
- Convert YAML to JSON-compatible values before the existing JCS canonicalizer so JSON and YAML inputs produce the same AWS `SecretString`.
- Normalize YAML suffixes to the legacy `.sops.json` source identity so encoding-only renames retain ownership.
- Accept exactly one top-level mapping and reject YAML features that cannot be represented unambiguously as JSON.

## Changes
- Source discovery and domain mapping now recognize `.sops.json`, `.sops.yaml`, and `.sops.yml`.
- The SOPS adapter decrypts YAML with MAC verification and enforces the JSON-compatible document contract.
- Tests, CLI help, Action metadata, README, and operator documentation describe and verify both encodings.

## Open Threads
- No release was made; the feature will ship through the normal future release process.

## Lessons
- Encoding-stable ownership can be preserved without changing the AWS model by normalizing only the filename suffix before hashing source identity.

## References
- [PR #13](https://github.com/meigma/sops-aws-sync/pull/13)
- Reviewed head `596570a75fa4b78bc67ff2daa226d43c534f2d56`; merge commit `f602168fada2a8fe8e82ba6116bf3492eae1dbe1`
- `.journal/009/YAML_SUPPORT_PLAN.md`

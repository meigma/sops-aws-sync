# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Session 001's `.journal/001/DESIGN.md` is the sole V1 product and architecture authority. Its `.journal/001/PLAN.md` is the subordinate five-phase delivery roadmap and must not add or change design requirements.
- Session 002 completed the accepted disposable Phase 1 proof in PR #7, which was closed without merge. It verified exact-commit go-git reads, stable in-process SOPS JSON decryption with MAC enforcement, strict JSON plus JCS canonicalization, deterministic mapping, and a pure `create` plan without an AWS client. Consult `.journal/002/SUMMARY.md` for evidence and findings.
- Session 003 landed Phase 2 in PR #8 at merge commit `f3f995e`: `master` now contains the durable exact-commit Git/SOPS-to-AWS create/update reconciliation slice, while restore, deletion, enumeration, and lifecycle policy remain Phase 3 work. Consult `.journal/003/SUMMARY.md` for the design decisions, review fixes, verification evidence, and live AWS acceptance result.
- The Phase 2 AWS acceptance test passed through whzbox in `us-east-1`, proving create → no-op → external-drift update → no-op against genuine Secrets Manager; use the opt-in `TestAWSSandboxCreateUpdateNoOpAndVerification` test for future live regression checks.
- On this workstation, if Moon prepends the global Proto Go over mise's pinned Go and reports a compiler/GOROOT version mismatch, run `MOON_TOOLCHAIN_FORCE_GLOBALS=true mise exec -- moon ...`; do not change repository pins for that local collision.

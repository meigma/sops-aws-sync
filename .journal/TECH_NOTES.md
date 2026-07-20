# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Session 001's `.journal/001/DESIGN.md` is the sole V1 product and architecture authority. Its `.journal/001/PLAN.md` is the subordinate five-phase delivery roadmap and must not add or change design requirements.
- Session 002 completed the accepted disposable Phase 1 proof in PR #7, which was closed without merge. It verified exact-commit go-git reads, stable in-process SOPS JSON decryption with MAC enforcement, strict JSON plus JCS canonicalization, deterministic mapping, and a pure `create` plan without an AWS client. Consult `.journal/002/SUMMARY.md` for evidence and findings.
- Durable product implementation has not started: `master` remains at the template's initial commit. Phase 2 is next and must begin from fetched `master` in a fresh session/worktree after rereading session 001's design and plan; PR #7 is evidence, not landed code.
- On this workstation, if Moon prepends the global Proto Go over mise's pinned Go and reports a compiler/GOROOT version mismatch, run `MOON_TOOLCHAIN_FORCE_GLOBALS=true mise exec -- moon ...`; do not change repository pins for that local collision.

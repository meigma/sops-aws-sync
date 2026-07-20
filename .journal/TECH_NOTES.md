# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Session 001's `.journal/001/DESIGN.md` is the sole V1 product and architecture authority. Its `.journal/001/PLAN.md` is the subordinate five-phase delivery roadmap and must not add or change design requirements.
- Product implementation has not started. Begin with the disposable Phase 1 proof only after explicit user approval, and use `.journal/001/SUMMARY.md` as the cold-start artifact index.

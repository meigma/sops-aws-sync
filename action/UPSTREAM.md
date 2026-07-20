# Upstream TypeScript Action baseline

The Action started from
[`actions/typescript-action` commit `57b9acc0d972b482f0db345fa09703f3612fda95`](https://github.com/actions/typescript-action/tree/57b9acc0d972b482f0db345fa09703f3612fda95),
the exact snapshot selected by the V1 design.

## Copied baseline files

- `.prettierignore`
- `.prettierrc.yml`
- `.env.example`
- `eslint.config.mjs`
- `jest.config.js`
- `package.json`
- `package-lock.json`
- `rollup.config.ts`
- `tsconfig.json`

The Node 24, ESM, NodeNext, ES2022, strict TypeScript, Jest, ESLint, Prettier,
and Rollup choices remain intact. The files were relocated beneath `action/` and
adapted to the repository's Moon and mise layout.

## Deliberate deviations

- Root `action.yml` defines the `sops-aws-sync` contract and points at
  `action/dist/index.js`.
- Template sample source, fixtures, tests, metadata, release scripts, workflows,
  and documentation were not copied because this repository already owns those
  surfaces or the sample behavior is unrelated.
- The package metadata and dependencies describe the thin CLI adapter.
- The canonical local-action tool remains available for manual source-level runs
  after copying `.env.example` to the ignored `.env` file.
- Node 24 is pinned by root `mise.toml` and `mise.lock` instead of a copied
  `.node-version` file.
- Moon owns format, lint, explicit typecheck, Jest, coverage, bundle,
  dependency, and committed-distribution checks.

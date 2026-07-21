import { readFile } from 'node:fs/promises'

const lock = JSON.parse(
  await readFile(new URL('../package-lock.json', import.meta.url), 'utf8')
)
const packageMetadata = JSON.parse(
  await readFile(new URL('../package.json', import.meta.url), 'utf8')
)
const releaseManifest = JSON.parse(
  await readFile(
    new URL('../../.release-please-manifest.json', import.meta.url),
    'utf8'
  )
)
const releaseConfig = JSON.parse(
  await readFile(
    new URL('../../release-please-config.json', import.meta.url),
    'utf8'
  )
)
const actionMetadata = await readFile(
  new URL('../../action.yml', import.meta.url),
  'utf8'
)
const packages = Object.keys(lock.packages ?? {})
const forbidden = packages.filter(
  (name) =>
    name.includes('node_modules/@aws-sdk/') ||
    name.endsWith('node_modules/aws-sdk')
)

if (forbidden.length > 0) {
  throw new Error('Action dependency graph must not contain an AWS SDK')
}

const compatibleVersion =
  /^ {4}default: ([^\s#]+) # x-release-please-version$/m.exec(
    actionMetadata
  )?.[1]
const repositoryVersion =
  releaseManifest['.'] ?? releaseConfig['initial-version']
if (
  packageMetadata.version !== repositoryVersion ||
  compatibleVersion !== packageMetadata.version
) {
  throw new Error(
    'Action package, metadata CLI, and repository release versions must match'
  )
}

const extraFiles = releaseConfig.packages?.['.']?.['extra-files'] ?? []
const expectedExtraFiles = [
  ['generic', 'action.yml', undefined],
  ['json', 'action/package.json', '$.version'],
  ['json', 'action/package-lock.json', '$.version'],
  ['json', 'action/package-lock.json', "$['packages'][''].version"]
]

for (const [type, path, jsonpath] of expectedExtraFiles) {
  const configured = extraFiles.some(
    (file) =>
      file.type === type &&
      file.path === path &&
      (jsonpath === undefined || file.jsonpath === jsonpath)
  )

  if (!configured) {
    throw new Error(
      `Release Please must update ${path}${jsonpath ? ` at ${jsonpath}` : ''}`
    )
  }
}

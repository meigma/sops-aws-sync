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
const inputsSource = await readFile(
  new URL('../src/inputs.ts', import.meta.url),
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

const compatibleVersion = /const compatibleCliVersion = '([^']+)'/.exec(
  inputsSource
)?.[1]
if (
  packageMetadata.version !== releaseManifest['.'] ||
  compatibleVersion !== packageMetadata.version
) {
  throw new Error(
    'Action package, embedded CLI, and repository release versions must match'
  )
}

import { createHash } from 'node:crypto'
import { chmod, mkdtemp, readFile, rm, stat } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import * as exec from '@actions/exec'
import { HttpClient } from '@actions/http-client'
import * as toolCache from '@actions/tool-cache'

import { ActionError } from './errors.js'
import type { ActionConfig } from './inputs.js'
import { AbsolutePath, type ExactVersion } from './types.js'

const repository = 'meigma/sops-aws-sync'
const signerWorkflow = `${repository}/.github/workflows/attest.yml`
const oidcIssuer = 'https://token.actions.githubusercontent.com'
const slsaPredicate = 'https://slsa.dev/provenance/v1'
const binaryName = 'sops-aws-sync'
const requiredAttestationFlags = [
  '--cert-oidc-issuer',
  '--deny-self-hosted-runners',
  '--digest-alg',
  '--format',
  '--predicate-type',
  '--repo',
  '--signer-digest',
  '--signer-workflow',
  '--source-digest',
  '--source-ref'
]

/** ReleaseAsset is one exact immutable GitHub release asset. */
export interface ReleaseAsset {
  readonly name: string
  readonly url: string
}

/** ReleaseMetadata binds one exact release tag to its immutable commit and assets. */
export interface ReleaseMetadata {
  readonly commit: string
  readonly assets: readonly ReleaseAsset[]
}

/** ReleaseProvider resolves only GitHub release and ref metadata. */
export interface ReleaseProvider {
  /** resolve binds an exact semantic version to one release commit and asset set. */
  resolve(
    version: ExactVersion,
    token: string | undefined
  ): Promise<ReleaseMetadata>
}

/** CacheProvider is the narrow verified-download and tool-cache boundary. */
export interface CacheProvider {
  /** find returns an existing exact cache directory or an empty string. */
  find(version: ExactVersion, architecture: string): string
  /** download fetches one exact release asset into the supplied destination. */
  download(
    asset: ReleaseAsset,
    destination: string,
    token: string | undefined
  ): Promise<string>
  /** cache stores one already verified executable under an exact key. */
  cache(
    file: string,
    version: ExactVersion,
    architecture: string
  ): Promise<string>
}

/** ProvenanceVerifier authenticates exact artifact bytes and release identity. */
export interface ProvenanceVerifier {
  /** verify fails closed unless GitHub attestation policy accepts the artifact. */
  verify(
    binary: AbsolutePath,
    version: ExactVersion,
    releaseCommit: string,
    digest: string,
    token: string | undefined
  ): Promise<void>
}

/** CommandOutput is the bounded result of one silent GitHub CLI invocation. */
export interface CommandOutput {
  readonly exitCode: number
  readonly stdout: string
}

/** GhCommand is the shell-free GitHub CLI execution boundary. */
export interface GhCommand {
  /** output invokes gh directly and captures output without command echo. */
  output(
    argumentsList: readonly string[],
    environment: Readonly<Record<string, string>>
  ): Promise<CommandOutput>
}

/** InstallerPort installs one verified exact CLI release. */
export interface InstallerPort {
  /** install returns the absolute verified executable path. */
  install(config: ActionConfig): Promise<AbsolutePath>
}

/** VerifiedInstaller enforces checksum and provenance on misses and cache hits. */
export class VerifiedInstaller implements InstallerPort {
  public constructor(
    private readonly releases: ReleaseProvider,
    private readonly cache: CacheProvider,
    private readonly provenance: ProvenanceVerifier
  ) {}

  /** install resolves, verifies, and caches one supported release binary. */
  public async install(config: ActionConfig): Promise<AbsolutePath> {
    const platform = mapPlatform(
      config.runnerOS,
      config.runnerArch,
      config.cliVersion
    )
    const metadata = await this.releases.resolve(
      config.cliVersion,
      config.githubToken
    )
    const binaryAsset = exactAsset(metadata.assets, platform.assetName)
    const checksumAsset = exactAsset(metadata.assets, 'checksums.txt')
    const temporaryDirectory = await mkdtemp(
      path.join(config.runnerTemp.value, 'sops-aws-sync-')
    )

    try {
      const checksumPath = await this.cache.download(
        checksumAsset,
        path.join(temporaryDirectory, 'checksums.txt'),
        config.githubToken
      )
      const expectedDigest = await checksumFor(checksumPath, platform.assetName)
      const cachedDirectory = this.cache.find(
        config.cliVersion,
        platform.cacheArchitecture
      )
      if (cachedDirectory.length > 0) {
        const cachedBinary = AbsolutePath.parse(
          path.join(cachedDirectory, binaryName),
          'Cached CLI path'
        )
        await verifyDigest(cachedBinary, expectedDigest)
        await this.provenance.verify(
          cachedBinary,
          config.cliVersion,
          metadata.commit,
          expectedDigest,
          config.githubToken
        )
        return cachedBinary
      }

      const downloaded = await this.cache.download(
        binaryAsset,
        path.join(temporaryDirectory, binaryName),
        config.githubToken
      )
      const downloadedBinary = AbsolutePath.parse(
        downloaded,
        'Downloaded CLI path'
      )
      await verifyDigest(downloadedBinary, expectedDigest)
      await this.provenance.verify(
        downloadedBinary,
        config.cliVersion,
        metadata.commit,
        expectedDigest,
        config.githubToken
      )
      await chmod(downloadedBinary.value, 0o755)
      const installedDirectory = await this.cache.cache(
        downloadedBinary.value,
        config.cliVersion,
        platform.cacheArchitecture
      )
      const installedBinary = AbsolutePath.parse(
        path.join(installedDirectory, binaryName),
        'Installed CLI path'
      )
      await verifyDigest(installedBinary, expectedDigest)

      return installedBinary
    } catch (error: unknown) {
      if (error instanceof ActionError) {
        throw error
      }
      throw new ActionError('Exact CLI installation or verification failed')
    } finally {
      await rm(temporaryDirectory, { recursive: true, force: true })
    }
  }
}

/** GitHubReleaseProvider resolves immutable release and tag metadata via GitHub's API. */
export class GitHubReleaseProvider implements ReleaseProvider {
  private readonly client = new HttpClient('sops-aws-sync-action')

  /** resolve validates the exact release tag and dereferences it to a commit. */
  public async resolve(
    version: ExactVersion,
    token: string | undefined
  ): Promise<ReleaseMetadata> {
    const headers = apiHeaders(token)
    const release = await this.getJson(
      `https://api.github.com/repos/${repository}/releases/tags/${encodeURIComponent(version.tag())}`,
      headers
    )
    if (stringProperty(release, 'tag_name', 'release') !== version.tag()) {
      throw new ActionError(
        'GitHub release tag does not match the requested CLI version'
      )
    }
    const rawAssets = property(release, 'assets')
    if (!Array.isArray(rawAssets)) {
      throw new ActionError('GitHub release assets are malformed')
    }
    const assets = rawAssets.map((candidate) => {
      const asset = record(candidate, 'release asset')
      return Object.freeze({
        name: stringProperty(asset, 'name', 'release asset'),
        url: httpsURL(stringProperty(asset, 'url', 'release asset'))
      })
    })
    const commit = await this.resolveTagCommit(version, headers)

    return Object.freeze({ commit, assets: Object.freeze(assets) })
  }

  /** resolveTagCommit follows annotated tags but accepts only a final commit object. */
  private async resolveTagCommit(
    version: ExactVersion,
    headers: Readonly<Record<string, string>>
  ): Promise<string> {
    const reference = await this.getJson(
      `https://api.github.com/repos/${repository}/git/ref/tags/${encodeURIComponent(version.tag())}`,
      headers
    )
    let object = record(property(reference, 'object'), 'Git tag object')
    for (let depth = 0; depth < 5; depth += 1) {
      const objectType = stringProperty(object, 'type', 'Git tag object')
      const digest = gitCommit(stringProperty(object, 'sha', 'Git tag object'))
      if (objectType === 'commit') {
        return digest
      }
      if (objectType !== 'tag') {
        throw new ActionError('Release tag does not resolve to a Git commit')
      }
      const tag = await this.getJson(
        `https://api.github.com/repos/${repository}/git/tags/${digest}`,
        headers
      )
      object = record(property(tag, 'object'), 'Annotated Git tag object')
    }

    throw new ActionError('Release tag indirection exceeds the supported bound')
  }

  /** getJson bounds API success and converts transport details to a safe failure. */
  private async getJson(
    url: string,
    headers: Readonly<Record<string, string>>
  ): Promise<object> {
    try {
      const response = await this.client.get(url, headers)
      if (response.message.statusCode !== 200) {
        throw new ActionError(
          'GitHub release metadata request was not successful'
        )
      }
      const body = await response.readBody()
      if (body.length === 0 || body.length > 1_048_576) {
        throw new ActionError('GitHub release metadata has an invalid size')
      }
      return record(JSON.parse(body), 'GitHub release metadata')
    } catch (error: unknown) {
      if (error instanceof ActionError) {
        throw error
      }
      throw new ActionError('GitHub release metadata could not be verified')
    }
  }
}

/** ActionsCacheProvider adapts @actions/tool-cache without adding fallback paths. */
export class ActionsCacheProvider implements CacheProvider {
  /** find looks up one exact version and architecture. */
  public find(version: ExactVersion, architecture: string): string {
    return toolCache.find(binaryName, version.value, architecture)
  }

  /** download fetches one API asset with optional private-repository authorization. */
  public async download(
    asset: ReleaseAsset,
    destination: string,
    token: string | undefined
  ): Promise<string> {
    return toolCache.downloadTool(
      asset.url,
      destination,
      token === undefined ? undefined : `Bearer ${token}`,
      {
        Accept: 'application/octet-stream',
        'X-GitHub-Api-Version': '2022-11-28'
      }
    )
  }

  /** cache stores the already verified file under the standard executable name. */
  public async cache(
    file: string,
    version: ExactVersion,
    architecture: string
  ): Promise<string> {
    return toolCache.cacheFile(
      file,
      binaryName,
      binaryName,
      version.value,
      architecture
    )
  }
}

/** GhProvenanceVerifier invokes compatible GitHub CLI policy without a shell. */
export class GhProvenanceVerifier implements ProvenanceVerifier {
  public constructor(
    private readonly command: GhCommand = new ActionsGhCommand()
  ) {}

  /** verify probes required capabilities, applies every identity flag, and parses JSON evidence. */
  public async verify(
    binary: AbsolutePath,
    version: ExactVersion,
    releaseCommit: string,
    digest: string,
    token: string | undefined
  ): Promise<void> {
    const configDirectory = await mkdtemp(
      path.join(os.tmpdir(), 'sops-aws-sync-gh-config-')
    )
    try {
      const commandEnvironment = tokenEnvironment(token, configDirectory)
      await requireCompatibleGh(this.command, commandEnvironment)
      const result = await this.command.output(
        [
          'attestation',
          'verify',
          binary.value,
          '--repo',
          repository,
          '--signer-workflow',
          signerWorkflow,
          '--source-ref',
          `refs/tags/${version.tag()}`,
          '--source-digest',
          releaseCommit,
          '--signer-digest',
          releaseCommit,
          '--cert-oidc-issuer',
          oidcIssuer,
          '--deny-self-hosted-runners',
          '--predicate-type',
          slsaPredicate,
          '--digest-alg',
          'sha256',
          '--format',
          'json'
        ],
        commandEnvironment
      )
      if (result.exitCode !== 0) {
        throw new ActionError('GitHub artifact provenance verification failed')
      }
      verifyAttestationOutput(result.stdout, digest)
    } finally {
      await rm(configDirectory, { recursive: true, force: true })
    }
  }
}

/** ActionsGhCommand adapts @actions/exec to one silent direct gh process. */
export class ActionsGhCommand implements GhCommand {
  /** output executes gh without a shell or raw argument echo. */
  public async output(
    argumentsList: readonly string[],
    environment: Readonly<Record<string, string>>
  ): Promise<CommandOutput> {
    const result = await exec.getExecOutput('gh', [...argumentsList], {
      env: environment,
      silent: true,
      ignoreReturnCode: true
    })

    return Object.freeze({ exitCode: result.exitCode, stdout: result.stdout })
  }
}

/** createInstaller constructs the production exact-release installer. */
export function createInstaller(): InstallerPort {
  return new VerifiedInstaller(
    new GitHubReleaseProvider(),
    new ActionsCacheProvider(),
    new GhProvenanceVerifier()
  )
}

/** mapPlatform binds supported hosted runner pairs to GoReleaser asset identity. */
export function mapPlatform(
  runnerOS: string,
  runnerArchitecture: string,
  version: ExactVersion
): Readonly<{ assetName: string; cacheArchitecture: string }> {
  let operatingSystem: string
  if (runnerOS === 'Linux') {
    operatingSystem = 'linux'
  } else if (runnerOS === 'macOS') {
    operatingSystem = 'darwin'
  } else {
    throw new ActionError(
      'The CLI installer supports only Linux and macOS runners'
    )
  }

  let architecture: string
  let cacheArchitecture: string
  if (runnerArchitecture === 'X64') {
    architecture = 'amd64'
    cacheArchitecture = 'x64'
  } else if (runnerArchitecture === 'ARM64') {
    architecture = 'arm64'
    cacheArchitecture = 'arm64'
  } else {
    throw new ActionError(
      'The CLI installer supports only X64 and ARM64 runners'
    )
  }

  return Object.freeze({
    assetName: `${binaryName}_${version.value}_${operatingSystem}_${architecture}`,
    cacheArchitecture
  })
}

/** exactAsset rejects missing and duplicate release identities. */
function exactAsset(
  assets: readonly ReleaseAsset[],
  name: string
): ReleaseAsset {
  const matches = assets.filter((asset) => asset.name === name)
  if (matches.length !== 1) {
    throw new ActionError(`Release must contain exactly one ${name} asset`)
  }

  return matches[0]
}

/** checksumFor reads one bounded checksums file and requires one exact asset entry. */
export async function checksumFor(
  checksumPath: string,
  assetName: string
): Promise<string> {
  const metadata = await stat(checksumPath)
  if (!metadata.isFile() || metadata.size === 0 || metadata.size > 1_048_576) {
    throw new ActionError('Release checksums file has an invalid size or type')
  }
  const body = await readFile(checksumPath, 'utf8')
  const matches: string[] = []
  for (const line of body.split('\n')) {
    const parsed = /^([a-f0-9]{64}) {2}(\S+)$/.exec(line.trimEnd())
    if (parsed !== null && parsed[2] === assetName) {
      matches.push(parsed[1])
    }
  }
  if (matches.length !== 1) {
    throw new ActionError(
      'Release checksums must contain one exact binary entry'
    )
  }

  return matches[0]
}

/** verifyDigest binds the exact executable bytes to the release checksum. */
async function verifyDigest(
  binary: AbsolutePath,
  expected: string
): Promise<void> {
  const metadata = await stat(binary.value)
  if (!metadata.isFile() || metadata.size === 0) {
    throw new ActionError('CLI binary has an invalid size or file type')
  }
  const digest = createHash('sha256')
    .update(await readFile(binary.value))
    .digest('hex')
  if (digest !== expected) {
    throw new ActionError('CLI binary checksum verification failed')
  }
}

/** requireCompatibleGh proves the installed CLI exposes every mandatory policy flag. */
async function requireCompatibleGh(
  command: GhCommand,
  environment: Readonly<Record<string, string>>
): Promise<void> {
  const version = await command.output(['--version'], environment)
  if (
    version.exitCode !== 0 ||
    !/^gh version \d+\.\d+\.\d+/m.test(version.stdout)
  ) {
    throw new ActionError(
      'A compatible GitHub CLI is required for attestation verification'
    )
  }
  const help = await command.output(
    ['attestation', 'verify', '--help'],
    environment
  )
  if (
    help.exitCode !== 0 ||
    requiredAttestationFlags.some(
      (requiredFlag) => !help.stdout.includes(requiredFlag)
    )
  ) {
    throw new ActionError(
      'GitHub CLI lacks required attestation policy capabilities'
    )
  }
}

/** verifyAttestationOutput requires non-empty SLSA evidence for the exact subject digest. */
export function verifyAttestationOutput(
  encoded: string,
  expectedDigest: string
): void {
  let decoded: unknown
  try {
    decoded = JSON.parse(encoded)
  } catch {
    throw new ActionError('GitHub attestation output is not valid JSON')
  }
  if (!Array.isArray(decoded) || decoded.length === 0) {
    throw new ActionError(
      'GitHub attestation output contains no verified statements'
    )
  }
  const accepted = decoded.some((entry) => {
    try {
      const verification = record(
        property(record(entry, 'attestation'), 'verificationResult'),
        'verification result'
      )
      const statement = record(
        property(verification, 'statement'),
        'attestation statement'
      )
      if (
        stringProperty(statement, 'predicateType', 'attestation statement') !==
        slsaPredicate
      ) {
        return false
      }
      const subjects = property(statement, 'subject')
      if (!Array.isArray(subjects)) {
        return false
      }
      return subjects.some((subjectValue) => {
        const subject = record(subjectValue, 'attestation subject')
        const digest = record(property(subject, 'digest'), 'attestation digest')
        return (
          stringProperty(digest, 'sha256', 'attestation digest') ===
          expectedDigest
        )
      })
    } catch {
      return false
    }
  })
  if (!accepted) {
    throw new ActionError(
      'GitHub attestation output does not bind the expected binary digest'
    )
  }
}

/** apiHeaders builds private-safe GitHub API headers without logging them. */
function apiHeaders(
  token: string | undefined
): Readonly<Record<string, string>> {
  const headers: Record<string, string> = {
    Accept: 'application/vnd.github+json',
    'X-GitHub-Api-Version': '2022-11-28'
  }
  if (token !== undefined) {
    headers.Authorization = `Bearer ${token}`
  }

  return Object.freeze(headers)
}

/** tokenEnvironment exposes the masked workflow token only to GitHub CLI verification. */
function tokenEnvironment(
  token: string | undefined,
  configDirectory: string
): Readonly<Record<string, string>> {
  const environment: Record<string, string> = {}
  const excluded = new Set([
    'GH_CONFIG_DIR',
    'GH_ENTERPRISE_TOKEN',
    'GH_HOST',
    'GH_TOKEN',
    'GITHUB_ENTERPRISE_TOKEN',
    'GITHUB_TOKEN'
  ])
  for (const [name, value] of Object.entries(process.env)) {
    if (value !== undefined && !excluded.has(name)) {
      environment[name] = value
    }
  }
  environment.GH_CONFIG_DIR = configDirectory
  if (token !== undefined) {
    environment.GH_TOKEN = token
  }

  return environment
}

/** httpsURL restricts downloads to GitHub's TLS API endpoint. */
function httpsURL(value: string): string {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    throw new ActionError('GitHub release asset URL is invalid')
  }
  if (parsed.protocol !== 'https:' || parsed.hostname !== 'api.github.com') {
    throw new ActionError('GitHub release asset URL has an unexpected origin')
  }

  return parsed.toString()
}

/** gitCommit validates one immutable full SHA-1 release commit. */
function gitCommit(value: string): string {
  if (!/^[a-f0-9]{40}$/.test(value)) {
    throw new ActionError('Release tag commit identity is invalid')
  }

  return value
}

/** record narrows one unknown JSON object. */
function record(value: unknown, label: string): object {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new ActionError(`${label} must be an object`)
  }

  return value
}

/** property reads one unknown object field without an assertion. */
function property(value: object, key: string): unknown {
  return Reflect.get(value, key)
}

/** stringProperty requires one non-empty bounded string. */
function stringProperty(value: object, key: string, label: string): string {
  const candidate = property(value, key)
  if (
    typeof candidate !== 'string' ||
    candidate.length === 0 ||
    candidate.length > 4096
  ) {
    throw new ActionError(`${label} field ${key} must be a string`)
  }

  return candidate
}

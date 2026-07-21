import { createHash } from 'node:crypto'
import { copyFile, mkdtemp, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { afterEach, describe, expect, it } from '@jest/globals'

import {
  GhProvenanceVerifier,
  VerifiedInstaller,
  checksumFor,
  mapPlatform,
  verifyAttestationOutput,
  type CacheProvider,
  type CommandOutput,
  type GhCommand,
  type ProvenanceVerifier,
  type ReleaseAsset,
  type ReleaseMetadata,
  type ReleaseProvider
} from '../src/installer.js'
import {
  readInputs,
  type ActionConfig,
  type InputReader
} from '../src/inputs.js'
import type { AbsolutePath, ExactVersion } from '../src/types.js'

const temporaryDirectories: string[] = []
const binaryContents = Buffer.from('verified binary bytes')
const binaryDigest = createHash('sha256').update(binaryContents).digest('hex')
const releaseCommit = 'a'.repeat(40)

class Reader implements InputReader {
  public getInput(name: string): string {
    if (name === 'secret-prefix') return '/test/scope'
    if (name === 'cli-version') return '1.2.3'
    if (name === 'github-token') return 'workflow-token'
    return ''
  }

  public setSecret(): void {}
}

class Releases implements ReleaseProvider {
  public readonly metadata: ReleaseMetadata = {
    commit: releaseCommit,
    assets: [
      {
        name: 'sops-aws-sync_1.2.3_linux_amd64',
        url: 'https://api.github.com/binary'
      },
      { name: 'checksums.txt', url: 'https://api.github.com/checksums' }
    ]
  }

  public async resolve(): Promise<ReleaseMetadata> {
    return this.metadata
  }
}

class Cache implements CacheProvider {
  public cachedDirectory = ''
  public readonly downloads: string[] = []

  public find(): string {
    return this.cachedDirectory
  }

  public async download(
    asset: ReleaseAsset,
    destination: string
  ): Promise<string> {
    this.downloads.push(asset.name)
    if (asset.name === 'checksums.txt') {
      await writeFile(
        destination,
        `${binaryDigest}  sops-aws-sync_1.2.3_linux_amd64\n`
      )
    } else {
      await writeFile(destination, binaryContents)
    }
    return destination
  }

  public async cache(file: string): Promise<string> {
    const directory = await makeTemporaryDirectory()
    await copyFile(file, path.join(directory, 'sops-aws-sync'))
    this.cachedDirectory = directory
    return directory
  }
}

class Provenance implements ProvenanceVerifier {
  public readonly calls: string[] = []
  public readonly tokens: string[] = []

  public async verify(
    binary: AbsolutePath,
    _version: ExactVersion,
    _commit: string,
    digest: string,
    token: string | undefined
  ): Promise<void> {
    this.calls.push(`${binary.value}:${digest}`)
    if (token !== undefined) this.tokens.push(token)
  }
}

class Commands implements GhCommand {
  public readonly calls: string[][] = []
  public readonly environments: Readonly<Record<string, string>>[] = []

  public constructor(private readonly verificationOutput: string) {}

  public async output(
    argumentsList: readonly string[],
    environment: Readonly<Record<string, string>>
  ): Promise<CommandOutput> {
    this.calls.push([...argumentsList])
    this.environments.push(environment)
    if (argumentsList[0] === '--version') {
      return { exitCode: 0, stdout: 'gh version 2.94.0' }
    }
    if (argumentsList.includes('--help')) {
      return {
        exitCode: 0,
        stdout:
          '--cert-oidc-issuer --deny-self-hosted-runners --digest-alg --format --predicate-type --repo --signer-digest --signer-workflow --source-digest --source-ref'
      }
    }
    return { exitCode: 0, stdout: this.verificationOutput }
  }
}

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  )
})

describe('verified CLI installer', () => {
  it('maps only supported GoReleaser runner pairs', () => {
    const version = config('/tmp').cliVersion
    expect(mapPlatform('Linux', 'X64', version)).toEqual({
      assetName: 'sops-aws-sync_1.2.3_linux_amd64',
      cacheArchitecture: 'x64'
    })
    expect(mapPlatform('macOS', 'ARM64', version)).toEqual({
      assetName: 'sops-aws-sync_1.2.3_darwin_arm64',
      cacheArchitecture: 'arm64'
    })
    expect(() => mapPlatform('Windows', 'X64', version)).toThrow()
    expect(() => mapPlatform('Linux', 'S390X', version)).toThrow()
  })

  it('verifies checksum and provenance before populating the cache', async () => {
    const runnerTemp = await makeTemporaryDirectory()
    const cache = new Cache()
    const provenance = new Provenance()
    const installed = await new VerifiedInstaller(
      new Releases(),
      cache,
      provenance
    ).install(config(runnerTemp))

    expect(installed.value).toBe(
      path.join(cache.cachedDirectory, 'sops-aws-sync')
    )
    expect(cache.downloads).toEqual([
      'checksums.txt',
      'sops-aws-sync_1.2.3_linux_amd64'
    ])
    expect(provenance.calls).toHaveLength(1)
    expect(provenance.tokens).toEqual(['workflow-token'])
  })

  it('re-verifies a cache hit and never falls back to a download', async () => {
    const runnerTemp = await makeTemporaryDirectory()
    const cache = new Cache()
    cache.cachedDirectory = await makeTemporaryDirectory()
    await writeFile(
      path.join(cache.cachedDirectory, 'sops-aws-sync'),
      binaryContents
    )
    const provenance = new Provenance()

    await new VerifiedInstaller(new Releases(), cache, provenance).install(
      config(runnerTemp)
    )

    expect(cache.downloads).toEqual(['checksums.txt'])
    expect(provenance.calls).toHaveLength(1)
  })

  it('fails a corrupt cache hit without downloading a fallback binary', async () => {
    const runnerTemp = await makeTemporaryDirectory()
    const cache = new Cache()
    cache.cachedDirectory = await makeTemporaryDirectory()
    await writeFile(
      path.join(cache.cachedDirectory, 'sops-aws-sync'),
      'corrupt'
    )

    await expect(
      new VerifiedInstaller(new Releases(), cache, new Provenance()).install(
        config(runnerTemp)
      )
    ).rejects.toThrow('checksum verification failed')
    expect(cache.downloads).toEqual(['checksums.txt'])
  })

  it('requires one exact checksum entry', async () => {
    const directory = await makeTemporaryDirectory()
    const checksums = path.join(directory, 'checksums.txt')
    await writeFile(checksums, `${binaryDigest}  wanted\n`)
    await expect(checksumFor(checksums, 'wanted')).resolves.toBe(binaryDigest)
    await expect(checksumFor(checksums, 'missing')).rejects.toThrow()
    await writeFile(
      checksums,
      `${binaryDigest}  wanted\n${binaryDigest}  wanted\n`
    )
    await expect(checksumFor(checksums, 'wanted')).rejects.toThrow()
  })
})

describe('GitHub attestation policy', () => {
  it('requires SLSA evidence for the exact subject digest', () => {
    const output = attestationOutput(binaryDigest)
    expect(() => verifyAttestationOutput(output, binaryDigest)).not.toThrow()
    expect(() => verifyAttestationOutput(output, 'b'.repeat(64))).toThrow()
    expect(() => verifyAttestationOutput('[]', binaryDigest)).toThrow()
  })

  it('passes the complete immutable identity policy directly to gh', async () => {
    const commands = new Commands(attestationOutput(binaryDigest))
    const verifier = new GhProvenanceVerifier(commands)
    const binary = await localBinary()
    const version = config('/tmp').cliVersion

    await verifier.verify(
      binary,
      version,
      releaseCommit,
      binaryDigest,
      'masked-token'
    )

    const verifyCall = commands.calls[2]
    expect(verifyCall).toContain('--repo')
    expect(verifyCall).toContain('meigma/sops-aws-sync')
    expect(verifyCall).toContain('--signer-workflow')
    expect(verifyCall).toContain(
      'meigma/sops-aws-sync/.github/workflows/attest.yml'
    )
    expect(verifyCall).toContain('--source-ref')
    expect(verifyCall).toContain('refs/tags/v1.2.3')
    expect(verifyCall).toContain('--source-digest')
    expect(verifyCall).toContain('--signer-digest')
    expect(verifyCall).toContain(releaseCommit)
    expect(verifyCall).toContain('--cert-oidc-issuer')
    expect(verifyCall).toContain('https://token.actions.githubusercontent.com')
    expect(verifyCall).toContain('--deny-self-hosted-runners')
    expect(verifyCall).toContain('--predicate-type')
    expect(verifyCall).toContain('https://slsa.dev/provenance/v1')
    expect(verifyCall).toContain('--digest-alg')
    expect(verifyCall).toContain('sha256')
    expect(verifyCall).toContain('--format')
    expect(verifyCall).toContain('json')
    expect(commands.environments[2]?.GH_TOKEN).toBe('masked-token')
    expect(commands.environments[2]?.GH_HOST).toBeUndefined()
    expect(commands.environments[2]?.GH_CONFIG_DIR).toContain(
      'sops-aws-sync-gh-config-'
    )
  })
})

function config(runnerTemp: string): ActionConfig {
  return readInputs(new Reader(), {
    GITHUB_REPOSITORY: 'meigma/sops-aws-sync',
    GITHUB_SHA: releaseCommit,
    GITHUB_WORKSPACE: '/workspace',
    RUNNER_ARCH: 'X64',
    RUNNER_OS: 'Linux',
    RUNNER_TEMP: runnerTemp
  })
}

async function makeTemporaryDirectory(): Promise<string> {
  const directory = await mkdtemp(
    path.join(os.tmpdir(), 'sops-aws-sync-action-test-')
  )
  temporaryDirectories.push(directory)
  return directory
}

async function localBinary(): Promise<AbsolutePath> {
  const directory = await makeTemporaryDirectory()
  const filename = path.join(directory, 'sops-aws-sync')
  await writeFile(filename, binaryContents)
  const { AbsolutePath: PathType } = await import('../src/types.js')
  return PathType.parse(filename, 'test binary')
}

function attestationOutput(digest: string): string {
  return JSON.stringify([
    {
      verificationResult: {
        statement: {
          predicateType: 'https://slsa.dev/provenance/v1',
          subject: [{ digest: { sha256: digest } }]
        }
      }
    }
  ])
}

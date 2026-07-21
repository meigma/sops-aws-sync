import { readFileSync } from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from '@jest/globals'

import { buildArguments } from '../src/command.js'
import {
  readInputs,
  type ActionEnvironment,
  type InputReader
} from '../src/inputs.js'
import {
  AbsolutePath,
  DurationValue,
  ExactVersion,
  RepositoryPath
} from '../src/types.js'

const environment: ActionEnvironment = {
  GITHUB_REPOSITORY: 'meigma/sops-aws-sync',
  GITHUB_SHA: '0123456789abcdef0123456789abcdef01234567',
  GITHUB_WORKSPACE: '/workspace/repository',
  RUNNER_ARCH: 'X64',
  RUNNER_OS: 'Linux',
  RUNNER_TEMP: '/runner/temp'
}

class Reader implements InputReader {
  public readonly masked: string[] = []

  public constructor(
    private readonly values: Readonly<Record<string, string>> = {}
  ) {}

  public getInput(name: string): string {
    if (name === 'github-token') {
      return this.values[name] ?? 'workflow-token'
    }
    return this.values[name] ?? ''
  }

  public setSecret(secret: string): void {
    this.masked.push(secret)
  }
}

describe('Action input and argv contract', () => {
  it('gives omitted github-token input the workflow token metadata default', () => {
    const metadata = readFileSync(path.resolve('..', 'action.yml'), 'utf8')
    const tokenInput = metadata.match(
      / {2}github-token:\n(?<block>(?: {4}.*\n)+)/
    )?.groups?.block

    expect(tokenInput).toContain('default: ${{ github.token }}')
  })

  it('parses typed defaults and constructs the exact minimum argv', () => {
    const reader = new Reader({ 'secret-prefix': '/acme/payments' })
    const config = readInputs(reader, environment)
    const reportPath = AbsolutePath.parse('/runner/temp/report.json', 'report')

    expect(config.mode).toBe('plan')
    expect(config.cliVersion.value).toBe('0.1.1')
    expect(config.githubToken).toBe('workflow-token')
    expect(reader.masked).toEqual(['workflow-token'])
    expect(buildArguments(config, reportPath)).toEqual([
      'plan',
      '--repository',
      '/workspace/repository',
      '--revision',
      '0123456789abcdef0123456789abcdef01234567',
      '--repository-id',
      'meigma/sops-aws-sync',
      '--source-root',
      'secrets',
      '--secret-prefix',
      '/acme/payments',
      '--log-format=json',
      '--report-file',
      '/runner/temp/report.json'
    ])
  })

  it('maps every supplied public control without an arbitrary args channel', () => {
    const reader = new Reader({
      mode: 'plan',
      'fail-on-drift': 'true',
      'source-root': 'config/secrets',
      'secret-prefix': '/production/payments',
      revision: 'release-commit',
      'repository-id': 'acme/payments',
      'aws-region': 'us-east-1',
      'recovery-window-days': '7',
      'run-timeout': '12m30s',
      'operation-timeout': '45s',
      'verification-timeout': '3m',
      'allow-empty': 'true',
      'show-resource-names': 'true',
      'cli-version': '1.2.3-rc.1',
      'github-token': 'masked-token'
    })
    const config = readInputs(reader, environment)
    const argumentsList = buildArguments(
      config,
      AbsolutePath.parse('/runner/temp/report.json', 'report')
    )

    expect(reader.masked).toEqual(['masked-token'])
    expect(argumentsList).toEqual([
      'plan',
      '--repository',
      '/workspace/repository',
      '--revision',
      'release-commit',
      '--repository-id',
      'acme/payments',
      '--source-root',
      'config/secrets',
      '--secret-prefix',
      '/production/payments',
      '--log-format=json',
      '--report-file',
      '/runner/temp/report.json',
      '--aws-region',
      'us-east-1',
      '--recovery-window-days',
      '7',
      '--run-timeout',
      '12m30s',
      '--operation-timeout',
      '45s',
      '--verification-timeout',
      '3m',
      '--allow-empty',
      '--show-resource-names',
      '--detailed-exit-code'
    ])
  })

  it.each([
    ['mode', { mode: 'apply', 'secret-prefix': '/scope' }],
    ['boolean', { 'allow-empty': 'yes', 'secret-prefix': '/scope' }],
    [
      'path traversal',
      { 'source-root': '../secrets', 'secret-prefix': '/scope' }
    ],
    ['mutable version', { 'cli-version': 'latest', 'secret-prefix': '/scope' }],
    [
      'partial integer',
      { 'recovery-window-days': '7days', 'secret-prefix': '/scope' }
    ],
    ['duration', { 'run-timeout': 'ten minutes', 'secret-prefix': '/scope' }],
    [
      'identity',
      { 'repository-id': 'repository-only', 'secret-prefix': '/scope' }
    ],
    [
      'sync drift flag',
      { mode: 'sync', 'fail-on-drift': 'true', 'secret-prefix': '/scope' }
    ]
  ])('rejects invalid %s input completely', (_label, values) => {
    expect(() => readInputs(new Reader(values), environment)).toThrow()
  })

  it('rejects missing trusted runner defaults before installation', () => {
    expect(() =>
      readInputs(new Reader({ 'secret-prefix': '/scope' }), {
        ...environment,
        GITHUB_SHA: undefined
      })
    ).toThrow('Runner environment GITHUB_SHA is required')
  })

  it('fails before installation when the metadata workflow-token default is absent', () => {
    expect(() =>
      readInputs(
        new Reader({
          'github-token': '',
          'secret-prefix': '/scope'
        }),
        environment
      )
    ).toThrow('did not receive a workflow token')
  })

  it('constructs strong value types without partial parsing', () => {
    expect(ExactVersion.parse('1.2.3').tag()).toBe('v1.2.3')
    expect(RepositoryPath.parse('.').value).toBe('.')
    expect(DurationValue.parse('1h2m3s', 'duration').value).toBe('1h2m3s')
    expect(() => ExactVersion.parse('v1.2.3')).toThrow()
    expect(() => RepositoryPath.parse('/absolute')).toThrow()
    expect(() => DurationValue.parse('0s', 'duration')).toThrow()
  })
})

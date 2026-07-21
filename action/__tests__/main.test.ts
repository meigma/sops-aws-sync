import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { afterEach, describe, expect, it } from '@jest/globals'

import type { InstallerPort } from '../src/installer.js'
import type { ActionEnvironment } from '../src/inputs.js'
import { runAction, type ActionIO } from '../src/main.js'
import {
  ActionsCliExecutor,
  type CliExecutor,
  type ProcessLog
} from '../src/process.js'
import type { ActionReport } from '../src/report.js'
import { AbsolutePath } from '../src/types.js'

const temporaryDirectories: string[] = []

class IO implements ActionIO {
  public readonly inputs = new Map<string, string>()
  public readonly masked: string[] = []
  public readonly information: string[] = []
  public readonly warnings: string[] = []
  public readonly outputs = new Map<string, string | number>()
  public readonly failures: string[] = []
  public readonly summaries: ActionReport[] = []

  public getInput(name: string): string {
    if (name === 'github-token') {
      return this.inputs.get(name) ?? 'workflow-token'
    }
    if (name === 'cli-version') {
      return this.inputs.get(name) ?? '0.1.1'
    }
    return this.inputs.get(name) ?? ''
  }

  public setSecret(secret: string): void {
    this.masked.push(secret)
  }

  public info(message: string): void {
    this.information.push(message)
  }

  public warning(message: string): void {
    this.warnings.push(message)
  }

  public setOutput(name: string, value: string | number): void {
    this.outputs.set(name, value)
  }

  public setFailed(message: string): void {
    this.failures.push(message)
  }

  public async writeSummary(report: ActionReport): Promise<void> {
    this.summaries.push(report)
  }
}

class Installer implements InstallerPort {
  public constructor(private readonly binary = '/tmp/sops-aws-sync') {}

  public async install(): Promise<AbsolutePath> {
    return AbsolutePath.parse(this.binary, 'test binary')
  }
}

class Executor implements CliExecutor {
  public readonly calls: readonly string[][] = []
  private readonly mutableCalls: string[][] = []

  public constructor(
    private readonly exitCode: number,
    private readonly report: object
  ) {
    this.calls = this.mutableCalls
  }

  public async execute(
    _binary: AbsolutePath,
    argumentsList: readonly string[],
    _workspace: AbsolutePath,
    log: ProcessLog
  ): Promise<number> {
    this.mutableCalls.push([...argumentsList])
    const reportFlag = argumentsList.indexOf('--report-file')
    const reportPath = argumentsList[reportFlag + 1]
    if (reportFlag < 0 || reportPath === undefined) {
      throw new Error('test command must contain a report path')
    }
    await writeFile(reportPath, JSON.stringify(this.report), { mode: 0o600 })
    log.info('{"level":"info","msg":"safe"}')
    return this.exitCode
  }
}

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  )
})

describe('Action orchestration', () => {
  it('publishes only validated report metadata on success', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()
    const executor = new Executor(0, report('drift'))

    await runAction(io, environment(temporary), {
      installer: new Installer(),
      executor
    })

    expect(io.failures).toEqual([])
    expect(io.outputs.get('status')).toBe('drift')
    expect(io.outputs.get('create-count')).toBe(1)
    expect(io.outputs.get('redacted-report-path')).toEqual(
      expect.stringContaining(temporary)
    )
    expect(io.summaries).toHaveLength(1)
    expect(executor.calls[0]).toContain('--log-format=json')
  })

  it('maps plan drift to a fixed failed step after publishing the report', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()
    io.inputs.set('fail-on-drift', 'true')

    await runAction(io, environment(temporary), {
      installer: new Installer(),
      executor: new Executor(2, report('drift'))
    })

    expect(io.outputs.get('status')).toBe('drift')
    expect(io.failures).toEqual(['sops-aws-sync plan detected drift'])
  })

  it('never echoes secret-bearing inputs or the GitHub token', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()
    io.inputs.set('secret-prefix', '/sentinel/secret/prefix')
    io.inputs.set('github-token', 'sentinel-github-token')

    await runAction(io, environment(temporary), {
      installer: new Installer(),
      executor: new Executor(0, report('converged'))
    })

    expect(io.masked).toEqual(['sentinel-github-token'])
    expect(
      JSON.stringify([io.information, io.warnings, io.outputs, io.failures])
    ).not.toContain('sentinel')
  })

  it('normalizes unknown adapter errors to one safe failure', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()
    const installer: InstallerPort = {
      install: async () => {
        throw new Error('raw infrastructure secret sentinel')
      }
    }

    await runAction(io, environment(temporary), {
      installer,
      executor: new Executor(0, report('converged'))
    })

    expect(io.failures).toEqual(['sops-aws-sync Action execution failed'])
  })

  it.each([
    [3, 'invalid', 'configuration or desired state'],
    [4, 'conflict', 'ownership or observed-state conflict'],
    [5, 'apply-failed', 'apply did not complete safely'],
    [5, 'observation-failed', 'apply did not complete safely'],
    [6, 'verification-failed', 'verification did not converge'],
    [6, 'verification-inconclusive', 'verification did not converge'],
    [130, 'interrupted', 'was interrupted']
  ])(
    'propagates CLI exit %i with stable %s metadata',
    async (exitCode, status, message) => {
      const temporary = await makeTemporaryDirectory()
      const io = standardIO()

      await runAction(io, environment(temporary), {
        installer: new Installer(),
        executor: new Executor(exitCode, report(status))
      })

      expect(io.outputs.get('status')).toBe(status)
      expect(io.failures.join(' ')).toContain(message)
    }
  )

  it('fails closed when process status and report metadata disagree', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()

    await runAction(io, environment(temporary), {
      installer: new Installer(),
      executor: new Executor(0, report('invalid'))
    })

    expect(io.outputs.size).toBe(0)
    expect(io.failures).toEqual([
      'CLI exit status and redacted report disagree'
    ])
  })

  it('forwards stdout lines but replaces raw stderr with a fixed diagnostic', async () => {
    const temporary = await makeTemporaryDirectory()
    const script = path.join(temporary, 'process-test.mjs')
    await writeFile(
      script,
      "process.stdout.write('{\"safe\":true}\\n'); process.stderr.write('sentinel stderr\\n')"
    )
    const io = standardIO()
    const executor = new ActionsCliExecutor()
    const exitCode = await executor.execute(
      AbsolutePath.parse(process.execPath, 'node'),
      [script],
      AbsolutePath.parse(temporary, 'workspace'),
      io
    )

    expect(exitCode).toBe(0)
    expect(io.information).toEqual(['{"safe":true}'])
    expect(io.warnings).toEqual(['sops-aws-sync wrote a diagnostic to stderr'])
    expect(io.warnings.join(' ')).not.toContain('sentinel')
  })

  it('invokes the real Go CLI through the local Action process boundary', async () => {
    const temporary = await makeTemporaryDirectory()
    const io = standardIO()
    io.inputs.set('source-root', 'missing-functional-source')
    const binary = path.resolve('..', 'bin', 'sops-aws-sync')

    await runAction(io, environment(temporary), {
      installer: new Installer(binary),
      executor: new ActionsCliExecutor()
    })

    expect(io.outputs.get('status')).toBe('invalid')
    expect(io.failures).toEqual([
      'sops-aws-sync rejected the Action configuration or desired state'
    ])
    expect(io.information.join('\n')).not.toContain('/functional/scope')
  }, 30_000)
})

function standardIO(): IO {
  const io = new IO()
  io.inputs.set('secret-prefix', '/functional/scope')
  return io
}

function environment(temporary: string): ActionEnvironment {
  return {
    GITHUB_REPOSITORY: 'meigma/sops-aws-sync',
    GITHUB_SHA: '0123456789abcdef0123456789abcdef01234567',
    GITHUB_WORKSPACE: path.resolve('..'),
    RUNNER_ARCH: 'X64',
    RUNNER_OS: 'Linux',
    RUNNER_TEMP: temporary
  }
}

function report(status: string): object {
  return {
    schema_version: 'sops-aws-sync/report/v1',
    tool_version: '1.2.3',
    git_revision: '0123456789abcdef0123456789abcdef01234567',
    status,
    counts: {
      create: status === 'drift' ? 1 : 0,
      update: 0,
      restore: 0,
      schedule_delete: 0,
      unchanged: status === 'converged' ? 1 : 0,
      conflicts: 0
    },
    verification: status === 'converged' ? 'converged' : 'not-run',
    duration_ms: 1
  }
}

async function makeTemporaryDirectory(): Promise<string> {
  const directory = await mkdtemp(
    path.join(os.tmpdir(), 'sops-aws-sync-main-test-')
  )
  temporaryDirectories.push(directory)
  return directory
}

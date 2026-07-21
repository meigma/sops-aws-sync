import { chmod, mkdtemp } from 'node:fs/promises'
import path from 'node:path'

import * as core from '@actions/core'

import { buildArguments } from './command.js'
import { ActionError, safeErrorMessage } from './errors.js'
import { createInstaller, type InstallerPort } from './installer.js'
import {
  readInputs,
  type ActionConfig,
  type ActionEnvironment,
  type InputReader
} from './inputs.js'
import {
  ActionsCliExecutor,
  type CliExecutor,
  type ProcessLog
} from './process.js'
import { readReport, type ActionReport } from './report.js'
import { AbsolutePath } from './types.js'

/** ActionIO is the complete safe Actions SDK surface consumed by orchestration. */
export interface ActionIO extends InputReader, ProcessLog {
  /** setOutput publishes one non-sensitive machine result. */
  setOutput(name: string, value: string | number): void
  /** setFailed marks the step failed with one normalized safe message. */
  setFailed(message: string): void
  /** writeSummary publishes only validated redacted report metadata. */
  writeSummary(report: ActionReport): Promise<void>
}

/** ActionDependencies keeps installation and process adapters testable. */
export interface ActionDependencies {
  readonly installer: InstallerPort
  readonly executor: CliExecutor
}

/** runAction executes the five design-defined Action responsibilities. */
export async function runAction(
  io: ActionIO,
  environment: ActionEnvironment,
  dependencies: ActionDependencies
): Promise<void> {
  try {
    const config = readInputs(io, environment)
    const binary = await dependencies.installer.install(config)
    const reportPath = await createReportPath(config)
    const argumentsList = buildArguments(config, reportPath)
    const exitCode = await dependencies.executor.execute(
      binary,
      argumentsList,
      config.workspace,
      io
    )
    const report = await readReport(reportPath)
    validateExit(config, exitCode, report)
    publishOutputs(io, report, reportPath)
    await io.writeSummary(report)
    const failure = exitFailure(exitCode)
    if (failure !== undefined) {
      io.setFailed(failure)
    }
  } catch (error: unknown) {
    io.setFailed(safeErrorMessage(error))
  }
}

/** run constructs the production adapters and executes the Action. */
export async function run(): Promise<void> {
  await runAction(
    productionIO(),
    {
      GITHUB_REPOSITORY: process.env.GITHUB_REPOSITORY,
      GITHUB_SHA: process.env.GITHUB_SHA,
      GITHUB_WORKSPACE: process.env.GITHUB_WORKSPACE,
      RUNNER_ARCH: process.env.RUNNER_ARCH,
      RUNNER_OS: process.env.RUNNER_OS,
      RUNNER_TEMP: process.env.RUNNER_TEMP
    },
    {
      installer: createInstaller(),
      executor: new ActionsCliExecutor()
    }
  )
}

/** createReportPath allocates a restrictive runner-temporary report directory. */
async function createReportPath(config: ActionConfig): Promise<AbsolutePath> {
  try {
    const directory = await mkdtemp(
      path.join(config.runnerTemp.value, 'sops-aws-sync-report-')
    )
    await chmod(directory, 0o700)
    return AbsolutePath.parse(
      path.join(directory, 'report.json'),
      'Temporary report path'
    )
  } catch {
    throw new ActionError(
      'Unable to allocate the temporary redacted report path'
    )
  }
}

/** publishOutputs emits only the report's schema-approved safe metadata. */
function publishOutputs(
  io: ActionIO,
  report: ActionReport,
  reportPath: AbsolutePath
): void {
  io.setOutput('status', report.status)
  io.setOutput('cli-version', report.toolVersion)
  io.setOutput('git-revision', report.gitRevision)
  io.setOutput('create-count', report.counts.create)
  io.setOutput('update-count', report.counts.update)
  io.setOutput('restore-count', report.counts.restore)
  io.setOutput('scheduled-delete-count', report.counts.scheduleDelete)
  io.setOutput('unchanged-count', report.counts.unchanged)
  io.setOutput('conflict-count', report.counts.conflicts)
  io.setOutput('verification-status', report.verification)
  io.setOutput('redacted-report-path', reportPath.value)
}

/** validateExit cross-checks the numeric compatibility surface with report status. */
function validateExit(
  config: ActionConfig,
  exitCode: number,
  report: ActionReport
): void {
  if (exitCode === 0) {
    if (
      (config.mode === 'sync' && report.status !== 'converged') ||
      (config.mode === 'plan' &&
        report.status !== 'converged' &&
        report.status !== 'drift')
    ) {
      throw new ActionError('CLI exit status and redacted report disagree')
    }
    return
  }
  if (
    exitCode === 2 &&
    config.mode === 'plan' &&
    config.failOnDrift &&
    report.status === 'drift'
  ) {
    return
  }
  const validFailure =
    (exitCode === 3 && report.status === 'invalid') ||
    (exitCode === 4 && report.status === 'conflict') ||
    (exitCode === 5 &&
      (report.status === 'apply-failed' ||
        report.status === 'observation-failed')) ||
    (exitCode === 6 &&
      (report.status === 'verification-failed' ||
        report.status === 'verification-inconclusive')) ||
    (exitCode === 130 && report.status === 'interrupted')
  if (!validFailure) {
    throw new ActionError('CLI exit status and redacted report disagree')
  }
}

/** exitFailure maps every supported nonzero status to a fixed non-sensitive message. */
function exitFailure(exitCode: number): string | undefined {
  switch (exitCode) {
    case 0:
      return undefined
    case 2:
      return 'sops-aws-sync plan detected drift'
    case 3:
      return 'sops-aws-sync rejected the Action configuration or desired state'
    case 4:
      return 'sops-aws-sync found an ownership or observed-state conflict'
    case 5:
      return 'sops-aws-sync apply did not complete safely'
    case 6:
      return 'sops-aws-sync verification did not converge'
    case 130:
      return 'sops-aws-sync was interrupted'
    default:
      return 'sops-aws-sync returned an unsupported exit status'
  }
}

/** productionIO is the narrow concrete @actions/core adapter. */
function productionIO(): ActionIO {
  return {
    getInput: (name) => core.getInput(name),
    setSecret: (secret) => core.setSecret(secret),
    info: (message) => core.info(message),
    warning: (message) => core.warning(message),
    setOutput: (name, value) => core.setOutput(name, value),
    setFailed: (message) => core.setFailed(message),
    writeSummary: async (report) => {
      await core.summary
        .addHeading('sops-aws-sync')
        .addTable([
          [
            { data: 'Status', header: true },
            { data: 'CLI version', header: true },
            { data: 'Git revision', header: true },
            { data: 'Verification', header: true }
          ],
          [
            report.status,
            report.toolVersion,
            report.gitRevision,
            report.verification
          ]
        ])
        .addTable([
          [
            { data: 'Create', header: true },
            { data: 'Update', header: true },
            { data: 'Restore', header: true },
            { data: 'Scheduled delete', header: true },
            { data: 'Unchanged', header: true },
            { data: 'Conflicts', header: true }
          ],
          [
            report.counts.create.toString(),
            report.counts.update.toString(),
            report.counts.restore.toString(),
            report.counts.scheduleDelete.toString(),
            report.counts.unchanged.toString(),
            report.counts.conflicts.toString()
          ]
        ])
        .write()
    }
  }
}

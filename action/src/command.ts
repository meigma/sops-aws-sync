import type { ActionConfig } from './inputs.js'
import type { AbsolutePath } from './types.js'

/** buildArguments constructs the complete shell-free CLI argument vector. */
export function buildArguments(
  config: ActionConfig,
  reportPath: AbsolutePath
): readonly string[] {
  const argumentsList = [
    config.mode,
    '--repository',
    config.workspace.value,
    '--revision',
    config.revision.value,
    '--repository-id',
    config.repositoryId.value,
    '--source-root',
    config.sourceRoot.value,
    '--secret-prefix',
    config.secretPrefix.value,
    '--log-format=json',
    '--report-file',
    reportPath.value
  ]

  appendValue(argumentsList, '--aws-region', config.awsRegion?.value)
  appendValue(
    argumentsList,
    '--recovery-window-days',
    config.recoveryWindowDays?.toString()
  )
  appendValue(argumentsList, '--run-timeout', config.runTimeout?.value)
  appendValue(
    argumentsList,
    '--operation-timeout',
    config.operationTimeout?.value
  )
  appendValue(
    argumentsList,
    '--verification-timeout',
    config.verificationTimeout?.value
  )
  appendFlag(argumentsList, '--allow-empty', config.allowEmpty)
  appendFlag(argumentsList, '--show-resource-names', config.showResourceNames)
  appendFlag(
    argumentsList,
    '--detailed-exit-code',
    config.mode === 'plan' && config.failOnDrift
  )

  return Object.freeze(argumentsList)
}

/** appendValue adds one explicitly supplied CLI control. */
function appendValue(
  argumentsList: string[],
  flag: string,
  value: string | undefined
): void {
  if (value !== undefined) {
    argumentsList.push(flag, value)
  }
}

/** appendFlag adds one true-only boolean CLI control. */
function appendFlag(
  argumentsList: string[],
  flag: string,
  enabled: boolean
): void {
  if (enabled) {
    argumentsList.push(flag)
  }
}

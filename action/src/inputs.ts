import { ActionError } from './errors.js'
import {
  AbsolutePath,
  DurationValue,
  ExactVersion,
  OpaqueInput,
  OptionalInput,
  RepositoryIdentity,
  RepositoryPath,
  parseMode,
  type ActionMode
} from './types.js'

const compatibleCliVersion = '0.1.1'

/** InputReader is the narrow Actions input and masking boundary. */
export interface InputReader {
  /** getInput reads one Action input without logging it. */
  getInput(name: string): string
  /** setSecret masks a token before any installation work. */
  setSecret(secret: string): void
}

/** ActionEnvironment contains only workflow identity and filesystem defaults. */
export interface ActionEnvironment {
  readonly GITHUB_REPOSITORY?: string
  readonly GITHUB_SHA?: string
  readonly GITHUB_WORKSPACE?: string
  readonly RUNNER_ARCH?: string
  readonly RUNNER_OS?: string
  readonly RUNNER_TEMP?: string
}

/** ActionConfig is the complete immutable public input model. */
export interface ActionConfig {
  readonly mode: ActionMode
  readonly failOnDrift: boolean
  readonly sourceRoot: RepositoryPath
  readonly secretPrefix: OpaqueInput
  readonly revision: OpaqueInput
  readonly repositoryId: RepositoryIdentity
  readonly awsRegion?: OptionalInput
  readonly recoveryWindowDays?: number
  readonly runTimeout?: DurationValue
  readonly operationTimeout?: DurationValue
  readonly verificationTimeout?: DurationValue
  readonly allowEmpty: boolean
  readonly showResourceNames: boolean
  readonly cliVersion: ExactVersion
  readonly githubToken?: string
  readonly workspace: AbsolutePath
  readonly runnerTemp: AbsolutePath
  readonly runnerOS: string
  readonly runnerArch: string
}

/** readInputs totally parses all Action inputs and workflow defaults. */
export function readInputs(
  reader: InputReader,
  environment: ActionEnvironment
): ActionConfig {
  const mode = parseMode(input(reader, 'mode', 'plan'))
  const failOnDrift = booleanInput(reader, 'fail-on-drift')
  if (mode === 'sync' && failOnDrift) {
    throw new ActionError('Input fail-on-drift is valid only in plan mode')
  }

  const token = input(reader, 'github-token')
  if (token.length > 0) {
    reader.setSecret(token)
  }

  const workspace = requiredEnvironment(
    environment.GITHUB_WORKSPACE,
    'GITHUB_WORKSPACE'
  )
  const runnerTemp = requiredEnvironment(environment.RUNNER_TEMP, 'RUNNER_TEMP')
  const revision = input(
    reader,
    'revision',
    requiredEnvironment(environment.GITHUB_SHA, 'GITHUB_SHA')
  )
  const repositoryId = input(
    reader,
    'repository-id',
    requiredEnvironment(environment.GITHUB_REPOSITORY, 'GITHUB_REPOSITORY')
  )
  const recoveryWindow = input(reader, 'recovery-window-days')
  const runTimeout = input(reader, 'run-timeout')
  const operationTimeout = input(reader, 'operation-timeout')
  const verificationTimeout = input(reader, 'verification-timeout')

  return Object.freeze({
    mode,
    failOnDrift,
    sourceRoot: RepositoryPath.parse(input(reader, 'source-root', 'secrets')),
    secretPrefix: OpaqueInput.parse(
      input(reader, 'secret-prefix'),
      'Input secret-prefix'
    ),
    revision: OpaqueInput.parse(revision, 'Input revision'),
    repositoryId: RepositoryIdentity.parse(repositoryId),
    awsRegion: OptionalInput.parse(
      input(reader, 'aws-region'),
      'Input aws-region'
    ),
    recoveryWindowDays:
      recoveryWindow.length === 0
        ? undefined
        : boundedInteger(recoveryWindow, 'Input recovery-window-days', 7, 30),
    runTimeout:
      runTimeout.length === 0
        ? undefined
        : DurationValue.parse(runTimeout, 'Input run-timeout'),
    operationTimeout:
      operationTimeout.length === 0
        ? undefined
        : DurationValue.parse(operationTimeout, 'Input operation-timeout'),
    verificationTimeout:
      verificationTimeout.length === 0
        ? undefined
        : DurationValue.parse(
            verificationTimeout,
            'Input verification-timeout'
          ),
    allowEmpty: booleanInput(reader, 'allow-empty'),
    showResourceNames: booleanInput(reader, 'show-resource-names'),
    cliVersion: ExactVersion.parse(
      input(reader, 'cli-version', compatibleCliVersion)
    ),
    githubToken: token.length === 0 ? undefined : token,
    workspace: AbsolutePath.parse(workspace, 'GITHUB_WORKSPACE'),
    runnerTemp: AbsolutePath.parse(runnerTemp, 'RUNNER_TEMP'),
    runnerOS: requiredEnvironment(environment.RUNNER_OS, 'RUNNER_OS'),
    runnerArch: requiredEnvironment(environment.RUNNER_ARCH, 'RUNNER_ARCH')
  })
}

/** input reads and trims one string without ever echoing it. */
function input(reader: InputReader, name: string, defaultValue = ''): string {
  const value = reader.getInput(name).trim()
  return value.length === 0 ? defaultValue : value
}

/** booleanInput accepts only the two explicit boolean spellings. */
function booleanInput(reader: InputReader, name: string): boolean {
  const value = input(reader, name, 'false')
  if (value === 'true') {
    return true
  }
  if (value === 'false') {
    return false
  }

  throw new ActionError(`Input ${name} must be true or false`)
}

/** boundedInteger rejects partial parsing, signs, fractions, and out-of-range values. */
function boundedInteger(
  value: string,
  label: string,
  minimum: number,
  maximum: number
): number {
  if (!/^\d+$/.test(value)) {
    throw new ActionError(
      `${label} must be an integer from ${minimum} through ${maximum}`
    )
  }
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed) || parsed < minimum || parsed > maximum) {
    throw new ActionError(
      `${label} must be an integer from ${minimum} through ${maximum}`
    )
  }

  return parsed
}

/** requiredEnvironment reads one trusted runner default or fails before installation. */
function requiredEnvironment(value: string | undefined, name: string): string {
  if (value === undefined || value.trim().length === 0) {
    throw new ActionError(`Runner environment ${name} is required`)
  }

  return value.trim()
}

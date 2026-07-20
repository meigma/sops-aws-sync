import { readFile, stat } from 'node:fs/promises'

import { ActionError } from './errors.js'
import type { AbsolutePath } from './types.js'

const reportSchema = 'sops-aws-sync/report/v1'
const reportKeys = [
  'schema_version',
  'tool_version',
  'git_revision',
  'status',
  'counts',
  'verification',
  'duration_ms'
]
const countKeys = [
  'create',
  'update',
  'restore',
  'schedule_delete',
  'unchanged',
  'conflicts'
]

/** ActionCounts is the immutable safe operation summary. */
export interface ActionCounts {
  readonly create: number
  readonly update: number
  readonly restore: number
  readonly scheduleDelete: number
  readonly unchanged: number
  readonly conflicts: number
}

/** ActionReport is the exact redacted Go report consumed by the Action. */
export interface ActionReport {
  readonly toolVersion: string
  readonly gitRevision: string
  readonly status: string
  readonly counts: ActionCounts
  readonly verification: string
  readonly durationMilliseconds: number
}

/** readReport bounds, parses, and validates the redacted report file. */
export async function readReport(
  reportPath: AbsolutePath
): Promise<ActionReport> {
  let reportStat
  let encoded
  try {
    reportStat = await stat(reportPath.value)
    if (
      !reportStat.isFile() ||
      reportStat.size === 0 ||
      reportStat.size > 65_536
    ) {
      throw new ActionError('CLI report has an invalid size or file type')
    }
    encoded = await readFile(reportPath.value, 'utf8')
  } catch (error: unknown) {
    if (error instanceof ActionError) {
      throw error
    }
    throw new ActionError('CLI did not produce a readable redacted report')
  }

  let decoded: unknown
  try {
    decoded = JSON.parse(encoded)
  } catch {
    throw new ActionError('CLI report is not valid JSON')
  }

  return parseReport(decoded)
}

/** parseReport enforces the complete schema and rejects unexpected fields. */
export function parseReport(value: unknown): ActionReport {
  const report = record(value, 'CLI report')
  exactKeys(report, reportKeys, 'CLI report')
  if (stringProperty(report, 'schema_version') !== reportSchema) {
    throw new ActionError('CLI report schema is incompatible')
  }
  const countsValue = record(property(report, 'counts'), 'CLI report counts')
  exactKeys(countsValue, countKeys, 'CLI report counts')
  const counts = Object.freeze({
    create: countProperty(countsValue, 'create'),
    update: countProperty(countsValue, 'update'),
    restore: countProperty(countsValue, 'restore'),
    scheduleDelete: countProperty(countsValue, 'schedule_delete'),
    unchanged: countProperty(countsValue, 'unchanged'),
    conflicts: countProperty(countsValue, 'conflicts')
  })

  return Object.freeze({
    toolVersion: safeString(
      stringProperty(report, 'tool_version'),
      'tool_version',
      false
    ),
    gitRevision: safeString(
      stringProperty(report, 'git_revision'),
      'git_revision',
      true
    ),
    status: safeString(stringProperty(report, 'status'), 'status', false),
    counts,
    verification: safeString(
      stringProperty(report, 'verification'),
      'verification',
      false
    ),
    durationMilliseconds: countProperty(report, 'duration_ms')
  })
}

/** record narrows one unknown JSON value without an assertion. */
function record(value: unknown, label: string): object {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new ActionError(`${label} must be an object`)
  }

  return value
}

/** property reads one JSON property through reflection. */
function property(value: object, key: string): unknown {
  return Reflect.get(value, key)
}

/** exactKeys prevents a future or unsafe field from silently reaching outputs. */
function exactKeys(
  value: object,
  expected: readonly string[],
  label: string
): void {
  const actual = Object.keys(value).sort()
  const wanted = [...expected].sort()
  if (
    actual.length !== wanted.length ||
    actual.some((key, index) => key !== wanted[index])
  ) {
    throw new ActionError(`${label} has unexpected fields`)
  }
}

/** stringProperty requires one JSON string field. */
function stringProperty(value: object, key: string): string {
  const candidate = property(value, key)
  if (typeof candidate !== 'string') {
    throw new ActionError(`CLI report field ${key} must be a string`)
  }

  return candidate
}

/** safeString prevents control characters and oversized output metadata. */
function safeString(value: string, key: string, allowEmpty: boolean): string {
  if (
    (!allowEmpty && value.length === 0) ||
    value.length > 1024 ||
    hasControl(value)
  ) {
    throw new ActionError(`CLI report field ${key} is invalid`)
  }

  return value
}

/** hasControl rejects unsafe control bytes in output metadata. */
function hasControl(value: string): boolean {
  return [...value].some((character) => {
    const codePoint = character.codePointAt(0)
    return codePoint !== undefined && (codePoint <= 31 || codePoint === 127)
  })
}

/** countProperty requires one non-negative safe integer. */
function countProperty(value: object, key: string): number {
  const candidate = property(value, key)
  if (
    typeof candidate !== 'number' ||
    !Number.isSafeInteger(candidate) ||
    candidate < 0
  ) {
    throw new ActionError(
      `CLI report field ${key} must be a non-negative integer`
    )
  }

  return candidate
}

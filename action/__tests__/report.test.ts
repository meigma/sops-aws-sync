import { describe, expect, it } from '@jest/globals'

import { parseReport } from '../src/report.js'

function validReport(): object {
  return {
    schema_version: 'sops-aws-sync/report/v1',
    tool_version: '1.2.3',
    git_revision: '0123456789abcdef0123456789abcdef01234567',
    status: 'converged',
    counts: {
      create: 1,
      update: 2,
      restore: 3,
      schedule_delete: 4,
      unchanged: 5,
      conflicts: 0
    },
    verification: 'converged',
    duration_ms: 42
  }
}

describe('redacted report contract', () => {
  it('parses only the safe stable report fields', () => {
    expect(parseReport(validReport())).toEqual({
      toolVersion: '1.2.3',
      gitRevision: '0123456789abcdef0123456789abcdef01234567',
      status: 'converged',
      counts: {
        create: 1,
        update: 2,
        restore: 3,
        scheduleDelete: 4,
        unchanged: 5,
        conflicts: 0
      },
      verification: 'converged',
      durationMilliseconds: 42
    })
  })

  it('allows an empty revision on safe preflight failure', () => {
    const report = validReport()
    Reflect.set(report, 'git_revision', '')
    expect(parseReport(report).gitRevision).toBe('')
  })

  it('rejects unknown fields that could carry resource or secret data', () => {
    const report = validReport()
    Reflect.set(report, 'secret_value', 'sentinel-secret')
    expect(() => parseReport(report)).toThrow('unexpected fields')
  })

  it.each([
    ['wrong schema', 'schema_version', 'future/report/v2'],
    ['control character', 'status', 'safe\nunsafe'],
    ['empty status', 'status', ''],
    ['wrong count type', 'counts', { create: '1' }]
  ])('rejects %s', (_label, key, value) => {
    const report = validReport()
    Reflect.set(report, key, value)
    expect(() => parseReport(report)).toThrow()
  })

  it('rejects negative, fractional, and unsafe counts', () => {
    for (const invalid of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
      const report = validReport()
      const counts = Reflect.get(report, 'counts')
      if (typeof counts !== 'object' || counts === null) {
        throw new Error('test fixture counts must be an object')
      }
      Reflect.set(counts, 'create', invalid)
      expect(() => parseReport(report)).toThrow('non-negative integer')
    }
  })
})

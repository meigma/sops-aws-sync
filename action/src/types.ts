import path from 'node:path'

import { ActionError } from './errors.js'

/** ActionMode is the complete public mutation-mode union. */
export type ActionMode = 'plan' | 'sync'

/** parseMode totally parses the public mode input. */
export function parseMode(value: string): ActionMode {
  if (value === 'plan' || value === 'sync') {
    return value
  }

  throw new ActionError('Input mode must be plan or sync')
}

/** ExactVersion is an immutable semantic release version without a mutable ref. */
export class ExactVersion {
  private constructor(public readonly value: string) {}

  /** parse accepts one exact SemVer release or prerelease without a leading v. */
  public static parse(value: string): ExactVersion {
    const semanticVersion =
      /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/
    if (!semanticVersion.test(value)) {
      throw new ActionError(
        'Input cli-version must be an exact semantic version'
      )
    }

    return new ExactVersion(value)
  }

  /** tag returns the immutable Git release tag for this version. */
  public tag(): string {
    return `v${this.value}`
  }
}

/** RepositoryPath is a canonical repository-relative POSIX directory. */
export class RepositoryPath {
  private constructor(public readonly value: string) {}

  /** parse rejects absolute, traversal, backslash, and non-canonical paths. */
  public static parse(value: string): RepositoryPath {
    if (
      value.length === 0 ||
      value.includes('\\') ||
      path.posix.isAbsolute(value) ||
      path.posix.normalize(value) !== value ||
      value === '..' ||
      value.startsWith('../')
    ) {
      throw new ActionError(
        'Input source-root must be a canonical repository-relative path'
      )
    }

    return new RepositoryPath(value)
  }
}

/** AbsolutePath is a validated absolute local filesystem path. */
export class AbsolutePath {
  private constructor(public readonly value: string) {}

  /** parse rejects missing and relative filesystem paths. */
  public static parse(value: string, label: string): AbsolutePath {
    if (value.length === 0 || !path.isAbsolute(value) || value.includes('\0')) {
      throw new ActionError(`${label} must be an absolute path`)
    }

    return new AbsolutePath(path.normalize(value))
  }
}

/** DurationValue is a positive Go-compatible duration made from explicit units. */
export class DurationValue {
  private constructor(public readonly value: string) {}

  /** parse accepts bounded syntax while leaving duration bounds to the Go CLI. */
  public static parse(value: string, label: string): DurationValue {
    const duration = /^(?=.*[1-9])(?:\d+(?:ns|us|ms|s|m|h))+$/
    if (!duration.test(value)) {
      throw new ActionError(`${label} must be a positive Go duration`)
    }

    return new DurationValue(value)
  }
}

/** RepositoryIdentity is the stable owner/repository ownership identity. */
export class RepositoryIdentity {
  private constructor(public readonly value: string) {}

  /** parse accepts one canonical GitHub owner/repository pair. */
  public static parse(value: string): RepositoryIdentity {
    if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(value)) {
      throw new ActionError(
        'Input repository-id must be an owner/repository identity'
      )
    }

    return new RepositoryIdentity(value)
  }
}

/** OpaqueInput is a non-empty, single-line argv value. */
export class OpaqueInput {
  private constructor(public readonly value: string) {}

  /** parse prevents control characters without reimplementing Go validation. */
  public static parse(value: string, label: string): OpaqueInput {
    if (value.length === 0 || value.length > 1024 || hasControl(value)) {
      throw new ActionError(`${label} must be a non-empty single-line value`)
    }

    return new OpaqueInput(value)
  }
}

/** OptionalInput is a present single-line argv value. */
export class OptionalInput {
  private constructor(public readonly value: string) {}

  /** parse validates one optional value when it is present. */
  public static parse(value: string, label: string): OptionalInput | undefined {
    if (value.length === 0) {
      return undefined
    }

    if (value.length > 1024 || hasControl(value)) {
      throw new ActionError(`${label} must be a single-line value`)
    }

    return new OptionalInput(value)
  }
}

/** hasControl rejects ASCII control bytes from values forwarded or published. */
function hasControl(value: string): boolean {
  return [...value].some((character) => {
    const codePoint = character.codePointAt(0)
    return codePoint !== undefined && (codePoint <= 31 || codePoint === 127)
  })
}

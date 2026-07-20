import * as exec from '@actions/exec'

import type { AbsolutePath } from './types.js'

/** ProcessLog receives only CLI-produced lines, never executable or argv echoes. */
export interface ProcessLog {
  /** info forwards one complete JSON operational log line. */
  info(message: string): void
  /** warning reports a fixed diagnostic when the CLI writes to stderr. */
  warning(message: string): void
}

/** CliExecutor is the direct shell-free process boundary. */
export interface CliExecutor {
  /** execute runs the absolute CLI path with an explicit argv and working directory. */
  execute(
    binary: AbsolutePath,
    argumentsList: readonly string[],
    workspace: AbsolutePath,
    log: ProcessLog
  ): Promise<number>
}

/** ActionsCliExecutor adapts @actions/exec with command echo disabled. */
export class ActionsCliExecutor implements CliExecutor {
  /** execute handles the numeric exit status explicitly and never asks a shell to parse input. */
  public async execute(
    binary: AbsolutePath,
    argumentsList: readonly string[],
    workspace: AbsolutePath,
    log: ProcessLog
  ): Promise<number> {
    let reportedStandardError = false
    return exec.exec(binary.value, [...argumentsList], {
      cwd: workspace.value,
      silent: true,
      ignoreReturnCode: true,
      listeners: {
        stdline: (line) => {
          if (line.length > 0) {
            log.info(line)
          }
        },
        errline: () => {
          if (!reportedStandardError) {
            reportedStandardError = true
            log.warning('sops-aws-sync wrote a diagnostic to stderr')
          }
        }
      }
    })
  }
}

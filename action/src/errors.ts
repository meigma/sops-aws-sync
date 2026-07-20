/** ActionError contains a message that is safe to publish through setFailed. */
export class ActionError extends Error {
  public constructor(message: string) {
    super(message)
    this.name = 'ActionError'
  }
}

/** safeErrorMessage prevents unknown infrastructure errors from reaching logs. */
export function safeErrorMessage(error: unknown): string {
  if (error instanceof ActionError) {
    return error.message
  }

  return 'sops-aws-sync Action execution failed'
}

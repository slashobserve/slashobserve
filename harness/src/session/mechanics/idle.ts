import { CallError } from "../types"

export class IdleWatch {
  private readonly timeoutMs: number
  private readonly controller = new AbortController()
  private timer: ReturnType<typeof setTimeout> | undefined
  private firedFlag = false

  constructor(timeoutMs: number) {
    this.timeoutMs = timeoutMs
  }

  get signal(): AbortSignal {
    return this.controller.signal
  }

  get fired(): boolean {
    return this.firedFlag
  }

  touch(): void {
    if (this.timeoutMs <= 0 || this.firedFlag) {
      return
    }
    clearTimeout(this.timer)
    this.timer = setTimeout(() => {
      this.firedFlag = true
      this.controller.abort(new Error("idle"))
    }, this.timeoutMs)
  }

  stop(): void {
    clearTimeout(this.timer)
    this.timer = undefined
  }

  failure(
    turn: AbortSignal,
    call: AbortSignal,
    provider: string
  ): CallError | undefined {
    if (!this.firedFlag || turn.aborted || call.aborted) {
      return undefined
    }
    return new CallError(provider, "call_timeout", 0, true)
  }
}

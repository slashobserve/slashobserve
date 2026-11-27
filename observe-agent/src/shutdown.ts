export const shutdownGraceMs = 5_000

export type ShutdownOutcome = "drained" | "forced"

export const exitCodes: Record<ShutdownOutcome, number> = {
  drained: 0,
  forced: 124,
}

export function shutdownOutcome(
  work: Promise<void>,
  stopping: AbortSignal,
  graceMs: number
): Promise<ShutdownOutcome> {
  return new Promise((resolve, reject) => {
    work.then(() => resolve("drained"), reject)
    stopping.addEventListener(
      "abort",
      () => setTimeout(() => resolve("forced"), graceMs),
      { once: true }
    )
  })
}

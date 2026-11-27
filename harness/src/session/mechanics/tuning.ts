import type { StopReason } from "../types"

export const minBuildMaxTokens = 8192

export type Tuning = { effort: string; maxTokens: number }

export function stepUp(effort: string): string {
  switch (effort) {
    case "low":
      return "medium"
    case "medium":
      return "high"
    default:
      return effort
  }
}

export function forCall(configured: Tuning, planning: boolean): Tuning {
  if (planning) {
    return {
      effort: stepUp(configured.effort),
      maxTokens: configured.maxTokens,
    }
  }
  return {
    effort: configured.effort,
    maxTokens: Math.min(
      configured.maxTokens,
      Math.max(Math.floor(configured.maxTokens / 2), minBuildMaxTokens)
    ),
  }
}

export function cutOffEarly(
  configured: Tuning,
  call: Tuning,
  stop: StopReason
): boolean {
  return stop === "max_tokens" && call.maxTokens < configured.maxTokens
}

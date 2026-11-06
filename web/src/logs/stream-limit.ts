import { ApiError } from "@/api/errors"

const seriesLimitPattern = /more than (\d+) series/

export function streamLimit(error: unknown): number | null {
  if (!(error instanceof ApiError) || error.code !== "limit_exceeded") {
    return null
  }
  const match = seriesLimitPattern.exec(error.message)
  return match === null ? null : Number(match[1])
}

export function streamLimitText(limit: number): string {
  return `More than ${limit.toLocaleString("en")} streams match. Narrow the selector.`
}

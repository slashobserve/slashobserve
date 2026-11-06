import { ApiError } from "@/api/errors"
import type { ObserveRequest } from "@/observe/queries"

const buildLoadFailedText = "This build could not be loaded."
const buildRequesterOnlyText = "Only the person who asked can watch this build."

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function buildPageNotice({
  requestError,
  hasRequest,
  buildError,
}: {
  requestError: unknown
  hasRequest: boolean
  buildError: unknown
}): string | undefined {
  if (!hasRequest && requestError != null) {
    return isNotFound(requestError)
      ? buildRequesterOnlyText
      : buildLoadFailedText
  }
  if (isNotFound(buildError)) return buildRequesterOnlyText
  return undefined
}

export function buildPageTitle({
  specTitle,
  buildTitle,
  dashboardTitle,
}: {
  specTitle: string | undefined
  buildTitle: string | null | undefined
  dashboardTitle: string | undefined
}): string | undefined {
  return specTitle ?? buildTitle ?? dashboardTitle ?? undefined
}

type RunState =
  "Building" | `Saved version ${number}` | "Saved" | "Stopped" | "Failed"

export function runStateText({
  status,
  failure_code,
  result_version,
}: Pick<
  ObserveRequest,
  "status" | "failure_code" | "result_version"
>): RunState {
  switch (status) {
    case "queued":
    case "running":
      return "Building"
    case "succeeded":
      return result_version === null
        ? "Saved"
        : `Saved version ${result_version}`
    case "failed":
      return failure_code === "canceled" ? "Stopped" : "Failed"
  }
}

export type BuildBodyState = "grid" | "nothing-saved" | "waiting"

export function buildBodyState({
  failed,
  savedPanels,
}: {
  failed: boolean
  savedPanels: number
}): BuildBodyState {
  if (savedPanels > 0) return "grid"
  return failed ? "nothing-saved" : "waiting"
}

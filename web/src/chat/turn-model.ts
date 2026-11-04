import type { ObserveRequest } from "@/observe/queries"

export function resultFinishedAt(
  request: Pick<ObserveRequest, "status" | "finished_at" | "build_saved_at">
): string | null {
  return request.status === "failed" && request.build_saved_at !== null
    ? request.build_saved_at
    : request.finished_at
}

export function showsWorkingRow(
  request: Pick<ObserveRequest, "status">,
  stalled: boolean
): boolean {
  return request.status !== "queued" && !stalled
}

import {
  type QueryClient,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, isRetryable, unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { maxPromptLength } from "@/prompts/rules"
import { sentence, type ObserveRequest } from "@/observe/queries"

export type BuildProgress = components["schemas"]["AgentBuildProgress"]

export type PanelTree = {
  panels: readonly { id: string; panels?: readonly { id: string }[] }[]
}

type SavedRequest = Pick<
  ObserveRequest,
  "status" | "build_saved_at" | "dashboard_id"
>

export function savedDashboardId(request: SavedRequest): string | null {
  return request.status === "succeeded" || request.build_saved_at !== null
    ? request.dashboard_id
    : null
}

export function answeredRequestId(
  request: Pick<ObserveRequest, "id" | "status" | "finding">
): string | null {
  return request.status === "succeeded" && request.finding !== null
    ? request.id
    : null
}

export function buildGone(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 404 &&
    error.code === "not_found"
  )
}

function refetchRequest(client: QueryClient, requestId: string) {
  const queryKey = observeKeys.request(requestId)
  const state = client.getQueryState(queryKey)
  const invalidatedFetchInFlight =
    state?.isInvalidated === true && state.fetchStatus === "fetching"
  return client.invalidateQueries(
    { queryKey, exact: true, refetchType: "all" },
    { cancelRefetch: !invalidatedFetchInFlight }
  )
}

function untilAborted(signal: AbortSignal): Promise<never> {
  return new Promise((_, reject) => {
    const stop = () => reject(signal.reason)
    if (signal.aborted) stop()
    else signal.addEventListener("abort", stop, { once: true })
  })
}

export async function readBuild<T>(
  client: QueryClient,
  requestId: string,
  signal: AbortSignal,
  read: () => Promise<T>
): Promise<T> {
  try {
    return await read()
  } catch (error) {
    if (!buildGone(error)) throw error
    await refetchRequest(client, requestId)
    const request = client.getQueryData<SavedRequest>(
      observeKeys.request(requestId)
    )
    if (request === undefined || savedDashboardId(request) === null) {
      throw error
    }
    return untilAborted(signal)
  }
}

export function buildQuery(requestId: string, saves: number) {
  return queryOptions({
    queryKey: observeKeys.build(requestId, saves),
    queryFn: ({ signal, client }) =>
      readBuild(client, requestId, signal, () =>
        api
          .GET("/observe/{id}/build", {
            params: { path: { id: requestId } },
            signal,
          })
          .then(unwrap)
          .then((body) => body.build)
      ),
    staleTime: Infinity,
    retry: (count, error) => count < 3 && isRetryable(error),
  })
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`
}

function panelCount(build: BuildProgress): string {
  return build.planned_panels > 0 && build.planned_panels >= build.panels
    ? `${build.panels} of ${build.planned_panels} panels`
    : plural(build.panels, "panel")
}

export function buildStatusText({
  status,
  build,
  failureCode,
  queuePosition = null,
  canceling = false,
  continued = false,
}: {
  status: ObserveRequest["status"]
  build: BuildProgress | null
  failureCode: string | null
  queuePosition?: number | null
  canceling?: boolean
  continued?: boolean
}): string {
  switch (status) {
    case "queued":
      if (canceling) return "Canceling"
      return queuePosition === null
        ? "Waiting for the agent"
        : `Queued, position ${queuePosition}`
    case "running":
      if (canceling) return "Canceling"
      if (build === null) return "Waiting for the agent"
      return `Building, ${panelCount(build)}`
    case "failed": {
      if (continued) return "Continued in a new run"
      const verb = failureCode === "canceled" ? "Canceled" : "Stopped"
      if (build === null || build.panels === 0) {
        return `${verb} before anything was built`
      }
      return build.planned_panels > 0 && build.planned_panels >= build.panels
        ? `${verb} at ${panelCount(build)}`
        : `${verb} with ${panelCount(build)}`
    }
    case "succeeded":
      return "Finished"
  }
}

export function newPanelIds(
  previous: PanelTree,
  next: PanelTree
): ReadonlySet<string> {
  const known = new Set<string>()
  for (const panel of previous.panels) {
    known.add(panel.id)
    for (const child of panel.panels ?? []) known.add(child.id)
  }
  const added = new Set<string>()
  for (const panel of next.panels) {
    if (!known.has(panel.id)) added.add(panel.id)
    for (const child of panel.panels ?? []) {
      if (!known.has(child.id)) added.add(child.id)
    }
  }
  return added
}

export function continueNoteError(raw: string): string | null {
  return [...raw.trim()].length > maxPromptLength
    ? `Use at most ${maxPromptLength} characters.`
    : null
}

const forbiddenText = "Only the person who asked can do this."

export function cancelErrorText(code: string, message = ""): string {
  switch (code) {
    case "forbidden":
      return forbiddenText
    case "request_finished":
      return "The run already finished."
    default:
      return message === ""
        ? "The run could not be canceled."
        : sentence(message)
  }
}

export function continueErrorText(code: string, message = ""): string {
  switch (code) {
    case "version_conflict":
      return "Someone saved a newer version after this build started, so it can't be continued. Open the dashboard and ask again."
    case "forbidden":
      return forbiddenText
    case "not_continuable":
      return "This build can no longer be continued."
    default:
      return message === ""
        ? "The build could not be continued."
        : sentence(message)
  }
}

export function saveBuildErrorText(code: string, message = ""): string {
  switch (code) {
    case "version_conflict":
      return "Someone saved a newer version after this build started, so it can't be saved. Open the dashboard and ask again."
    case "forbidden":
    case "forbidden_role":
      return "You need Editor access to save it."
    case "build_empty":
      return "Nothing was built yet."
    default:
      return message === ""
        ? "The build could not be saved."
        : sentence(message)
  }
}

export function useSaveBuild(requestId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observe/{id}/build/save", {
          params: { path: { id: requestId } },
        })
        .then(unwrap),
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({
        queryKey: observeKeys.request(requestId),
      })
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
      void queryClient.invalidateQueries({
        queryKey: dashboardKeys.latest(saved.dashboard_id),
      })
      void queryClient.invalidateQueries({
        queryKey: dashboardKeys.revisions(saved.dashboard_id),
      })
    },
  })
}

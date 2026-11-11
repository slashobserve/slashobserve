import { stringifySearch } from "./search"

const maxSlugLength = 60
const historySegment = "versions"

export function slugOf(title: string): string {
  const slug = title
    .replace(/[A-Z]/g, (letter) => letter.toLowerCase())
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, maxSlugLength)
    .replace(/-+$/, "")
  return slug === historySegment ? "" : slug
}

function slugParam(title: string | undefined): string | undefined {
  return title === undefined ? undefined : slugOf(title) || undefined
}

export function dashboardLink(dashboardId: string, title?: string) {
  return {
    to: "/d/$dashboardId/{-$slug}",
    params: { dashboardId, slug: slugParam(title) },
  } as const
}

function path(...segments: string[]): string {
  return "/" + segments.map(encodeURIComponent).join("/")
}

export function dashboardPath(dashboardId: string, title?: string): string {
  const slug = slugParam(title)
  return slug === undefined
    ? path("d", dashboardId)
    : path("d", dashboardId, slug)
}

export function folderPath(folderId: string): string {
  return path("f", folderId)
}

export function chatPath(requestId: string): string {
  return path("c", requestId)
}

export function buildPath(requestId: string): string {
  return path("b", requestId)
}

export function findingPath(findingId: string): string {
  return path("findings", findingId)
}

export function findingLink(findingId: string) {
  return { to: "/findings/$findingId", params: { findingId } } as const
}

export function observationPath(observationId: string): string {
  return path("o", observationId)
}

// The /o/$observationId route is not registered yet, so the router's typed `to` rejects it; the observation page plan removes the casts.
export function observationLink(observationId: string) {
  return {
    to: "/o/$observationId" as never,
    params: { observationId } as never,
  }
}

export function observationsLink(search?: { new?: "1" }) {
  return { to: "/observations", search: search ?? {} } as const
}

export type LogsLinkSearch = {
  ds?: string
  q?: string
  from?: string
  to?: string
}

export function logsUrl(search: LogsLinkSearch): string {
  return "/logs" + stringifySearch(search)
}

type TraceRange = { from: string; to: string }

function traceSearch(
  datasourceUid: string,
  range: TraceRange | undefined,
  span: string | undefined
) {
  return { ds: datasourceUid, from: range?.from, to: range?.to, span }
}

export function traceLink(
  traceId: string,
  datasourceUid: string,
  range?: TraceRange,
  span?: string
) {
  return {
    to: "/t/$traceId",
    params: { traceId },
    search: traceSearch(datasourceUid, range, span),
  } as const
}

export function traceUrl(
  traceId: string,
  datasourceUid: string,
  range?: TraceRange,
  span?: string
): string {
  return (
    path("t", traceId) +
    stringifySearch(traceSearch(datasourceUid, range, span))
  )
}

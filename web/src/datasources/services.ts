import type { LogsSearch } from "@/logs/search"
import { encodePills } from "@/logs/pills"
import type { TraceSearch } from "@/traces/search"

import type { DatasourceStatus } from "./queries"
import type { ExploreKind } from "./sources"

export type ServiceSignal = "logs" | "traces"

type SourceServices = {
  field: string
  names: string[]
  truncated: boolean
}

export type ServicesState =
  | { status: "loading" }
  | { status: "unavailable" }
  | ({ status: "ready" } & SourceServices)

const shownServices = 3

export function serviceSignal(kind: ExploreKind): ServiceSignal | null {
  return kind === "metrics" ? null : kind
}

export function servicesOfResponse(response: {
  field: string
  services: string[]
  truncated: boolean
}): ServicesState {
  return {
    status: "ready",
    field: response.field,
    names: response.services,
    truncated: response.truncated,
  }
}

function matchesServices(text: string, names: readonly string[]) {
  const needle = text.trim().toLowerCase()
  return names.some((name) => name.toLowerCase().includes(needle))
}

export function filterSources<T extends DatasourceStatus>(
  sources: readonly T[],
  services: ReadonlyMap<string, ServicesState>,
  text: string
): T[] {
  const needle = text.trim().toLowerCase()
  if (needle === "") return [...sources]
  return sources.filter(
    (source) =>
      source.name.toLowerCase().includes(needle) ||
      source.uid.toLowerCase().includes(needle) ||
      matchesServices(needle, namesOf(services.get(source.uid)))
  )
}

function namesOf(state: ServicesState | undefined): readonly string[] {
  return state?.status === "ready" ? state.names : []
}

export function splitServices(
  names: readonly string[],
  expanded: boolean
): { shown: string[]; hidden: number } {
  if (expanded || names.length <= shownServices) {
    return { shown: [...names], hidden: 0 }
  }
  return {
    shown: names.slice(0, shownServices),
    hidden: names.length - shownServices,
  }
}

export function serviceSearch(
  kind: "logs" | "traces",
  field: string,
  name: string
): LogsSearch | TraceSearch {
  if (kind === "traces") return { service: name }
  const f = encodePills([
    {
      id: "0",
      kind: "is",
      field,
      value: name,
      negate: false,
      pinned: false,
      paused: false,
    },
  ])
  return f === undefined ? {} : { f }
}

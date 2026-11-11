import type { LogsSearch } from "@/logs/search"
import type { TraceSearch } from "@/traces/search"

import type { SavedSearch } from "./queries"

export type SavedSearchLocation =
  { to: "/logs"; search: LogsSearch } | { to: "/traces"; search: TraceSearch }

export function savedSearchLocation(
  saved: Pick<
    SavedSearch,
    "kind" | "datasource_uid" | "query" | "filters" | "from" | "to"
  >
): SavedSearchLocation {
  const { datasource_uid: ds, filters, from, to } = saved
  const query = saved.query ?? undefined
  if (saved.kind === "logs") {
    const columns = filters.columns ?? []
    return {
      to: "/logs",
      search: {
        ds,
        q: query,
        cols: columns.length === 0 ? undefined : columns.join(","),
        from,
        to,
      },
    }
  }
  return {
    to: "/traces",
    search: {
      ds,
      service: filters.service,
      span: filters.span,
      min: filters.min,
      max: filters.max,
      errors: filters.errors === true ? "1" : undefined,
      q: query,
      from,
      to,
    },
  }
}

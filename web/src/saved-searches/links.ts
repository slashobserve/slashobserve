import { logsLink, metricsLink, tracesLink } from "@/lib/links"

import type { SavedSearch } from "./queries"

export type SavedSearchLocation =
  | ReturnType<typeof logsLink>
  | ReturnType<typeof tracesLink>
  | ReturnType<typeof metricsLink>

export function savedSearchLocation(
  saved: Pick<
    SavedSearch,
    "id" | "kind" | "datasource_uid" | "query" | "filters" | "from" | "to"
  >
): SavedSearchLocation {
  const { datasource_uid: uid, filters, from, to } = saved
  const query = saved.query ?? undefined
  if (saved.kind === "logs") {
    const columns = filters.columns ?? []
    return logsLink(uid, {
      q: query,
      cols: columns.length === 0 ? undefined : columns.join(","),
      from,
      to,
      saved: saved.id,
    })
  }
  if (saved.kind === "metrics") {
    return metricsLink(uid, {
      q: query,
      mode: filters.mode === "table" ? "table" : undefined,
      from,
      to,
      saved: saved.id,
    })
  }
  return tracesLink(uid, {
    service: filters.service,
    span: filters.span,
    min: filters.min,
    max: filters.max,
    errors: filters.errors === true ? "1" : undefined,
    q: query,
    from,
    to,
    saved: saved.id,
  })
}

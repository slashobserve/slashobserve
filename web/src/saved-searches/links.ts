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
      ...(query === undefined ? {} : { q: query }),
      ...(columns.length === 0 ? {} : { cols: columns.join(",") }),
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
    ...(filters.service === undefined ? {} : { service: filters.service }),
    ...(filters.span === undefined ? {} : { span: filters.span }),
    ...(filters.min === undefined ? {} : { min: filters.min }),
    ...(filters.max === undefined ? {} : { max: filters.max }),
    ...(filters.errors === true ? { errors: "1" } : {}),
    ...(query === undefined ? {} : { q: query }),
    from,
    to,
    saved: saved.id,
  })
}

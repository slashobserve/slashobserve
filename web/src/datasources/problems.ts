import { ApiError } from "@/api/errors"
import { loadProblemText } from "@/service-accounts/queries"

import { DatasourceError } from "./admin-queries"

export function saveProblemText(error: unknown): string {
  if (error instanceof DatasourceError) {
    switch (error.code) {
      case "invalid_datasource":
        return "Fix the fields marked below."
      case "secret_key_missing":
        return "Set SLASHOBSERVE_SECRET_KEY on the server and restart to store credentials, or save without them."
      case "secret_required":
        return "Enter every credential again, or clear it, when you change the address or username."
      case "datasource_changed":
        return "This datasource changed while you were editing it. Reload it and try again."
      case "uid_taken":
        return "Another datasource already uses this id."
      case "datasource_read_only":
        return "This datasource comes from the datasources file. Edit the file and restart the server."
      case "outbound_domain_not_allowed":
        return `${error.message} Allow the domain in Settings, Outbound access, then try again.`
    }
  }
  if (error instanceof ApiError && error.status === 404) {
    return "This datasource no longer exists."
  }
  return loadProblemText(error)
}

export type UserLine = {
  key: string
  text: string
  dashboard?: { id: string; title: string }
}

export function usersSummary(
  users: { kind: string; id: string; title: string }[],
  hidden: number
): UserLine[] {
  const lines: UserLine[] = users.map((user) =>
    user.kind === "dashboard"
      ? {
          key: user.id,
          text: `Dashboard: ${user.title}`,
          dashboard: { id: user.id, title: user.title },
        }
      : { key: user.id, text: `Saved search: ${user.title}` }
  )
  if (hidden > 0) {
    lines.push({
      key: "hidden",
      text:
        hidden === 1
          ? "1 more in a folder you cannot see"
          : `${hidden} more in folders you cannot see`,
    })
  }
  return lines
}

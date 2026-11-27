import type { ProblemTexts } from "@/api/errors"

export const saveProblems: ProblemTexts = {
  codes: {
    invalid_datasource: "Fix the fields marked below.",
    secret_key_missing:
      "Set SLASHOBSERVE_SECRET_KEY on the server and restart to store credentials, or save without them.",
    secret_required:
      "Enter every credential again, or clear it, when you change the address or username.",
    datasource_changed:
      "This datasource changed while you were editing it. Reload it and try again.",
    uid_taken: "Another datasource already uses this id.",
    datasource_read_only:
      "This datasource comes from the datasources file. Edit the file and restart the server.",
    outbound_domain_not_allowed: (error) =>
      `${error.message} Allow the domain in Settings, Outbound access, then try again.`,
  },
  notFound: "This datasource no longer exists.",
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

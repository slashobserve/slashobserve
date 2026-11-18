export type AskOn = { dashboardId: string; title: string }

const maxTitleShown = 40

export function askOnOf(finding: object): AskOn | null {
  const raw = (finding as { ask_on?: unknown }).ask_on
  if (typeof raw !== "object" || raw === null) return null
  const { dashboard_id: dashboardId, title } = raw as Record<string, unknown>
  if (typeof dashboardId !== "string" || dashboardId === "") return null
  if (typeof title !== "string") return null
  return { dashboardId, title }
}

export function askOnLabel(title: string): string {
  const shown =
    title.length > maxTitleShown
      ? `${title.slice(0, maxTitleShown - 3)}...`
      : title
  return `Ask this on ${shown}`
}

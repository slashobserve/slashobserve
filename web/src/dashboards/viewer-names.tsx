import { useDashboardViewers } from "./viewers"

const shown = 3

export function viewersLabel(names: readonly string[]): string {
  if (names.length <= shown) return names.join(", ")
  return `${names.slice(0, shown).join(", ")} and ${names.length - shown} more`
}

export function ViewerNames({ dashboardId }: { dashboardId: string }) {
  const viewers = useDashboardViewers(dashboardId)
  if (viewers.length === 0) return null
  const label = viewersLabel(viewers.map((viewer) => viewer.name))
  return (
    <span
      className="max-w-[32ch] truncate text-xs text-muted-foreground"
      title={viewers.map((viewer) => viewer.name).join(", ")}
    >
      Also viewing: {label}
    </span>
  )
}

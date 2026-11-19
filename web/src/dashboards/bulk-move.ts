export function splitByFolder<T extends { folder: { id: string } }>(
  dashboards: readonly T[],
  folderId: string
): { moving: T[]; staying: T[] } {
  const moving: T[] = []
  const staying: T[] = []
  for (const dashboard of dashboards) {
    ;(dashboard.folder.id === folderId ? staying : moving).push(dashboard)
  }
  return { moving, staying }
}

type Failure = { title: string; reason: string }

type BulkMove = {
  folder: string
  moved: number
  already: number
  failed: readonly Failure[]
}

export type BulkMoveResult = {
  kind: "success" | "info" | "error"
  text: string
}

function dashboardCount(count: number): string {
  return `${count} ${count === 1 ? "dashboard" : "dashboards"}`
}

function listOf(items: readonly string[]): string {
  if (items.length <= 2) return items.join(" or ")
  return `${items.slice(0, -1).join(", ")} or ${items[items.length - 1]}`
}

function alreadyThere(folder: string, count: number): string {
  if (count === 1) return `It is already in ${folder}.`
  if (count === 2) return `Both are already in ${folder}.`
  return `All ${count} are already in ${folder}.`
}

export function bulkMoveResult({
  folder,
  moved,
  already,
  failed,
}: BulkMove): BulkMoveResult {
  if (failed.length > 0) {
    const names = listOf(
      failed.map(
        (failure) => `${failure.title} (${failure.reason.replace(/\.$/, "")})`
      )
    )
    const lead =
      moved === 0
        ? "Could not move"
        : `Moved ${moved} of ${moved + failed.length} dashboards to ${folder}. Could not move`
    return { kind: "error", text: `${lead} ${names}.` }
  }
  if (moved === 0) return { kind: "info", text: alreadyThere(folder, already) }
  const text = `Moved ${dashboardCount(moved)} to ${folder}`
  if (already === 0) return { kind: "success", text }
  const was = already === 1 ? "was" : "were"
  return {
    kind: "success",
    text: `${text}. ${already} ${was} already there.`,
  }
}

import type { components } from "@/api/schema.gen"

export type GrafanaImportResult = components["schemas"]["GrafanaImportResult"]
type GrafanaImportSummary = components["schemas"]["GrafanaImportSummary"]

export type ResultGroups = {
  failed: GrafanaImportResult[]
  needsRepair: GrafanaImportResult[]
  imported: GrafanaImportResult[]
  skipped: GrafanaImportResult[]
}

function needsRepair(result: GrafanaImportResult): boolean {
  return result.unverified_panels + result.placeholder_panels > 0
}

export function groupResults(results: GrafanaImportResult[]): ResultGroups {
  const groups: ResultGroups = {
    failed: [],
    needsRepair: [],
    imported: [],
    skipped: [],
  }
  for (const result of results) {
    switch (result.outcome) {
      case "failed":
        groups.failed.push(result)
        break
      case "skipped":
        groups.skipped.push(result)
        break
      case "imported":
      case "updated":
        if (needsRepair(result)) groups.needsRepair.push(result)
        else groups.imported.push(result)
    }
  }
  return groups
}

export function summaryText(
  summary: Pick<GrafanaImportSummary, "done" | "failed" | "skipped">
): string {
  const imported = summary.done - summary.failed - summary.skipped
  return `${imported} imported, ${summary.failed} failed, ${summary.skipped} skipped`
}

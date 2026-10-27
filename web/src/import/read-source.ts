import type { components } from "@/api/schema.gen"

export const MAX_IMPORT_BYTES = 8 * 1024 * 1024
export const MAX_FILE_BYTES = 64 * 1024 * 1024
export const TOO_LARGE_MESSAGE =
  "This file is larger than 8 MiB, the import limit."

type Summary = components["schemas"]["ImportSummary"]

export function requestBytes(
  text: string,
  mapping: Record<string, string>
): number {
  return new TextEncoder().encode(JSON.stringify({ json: text, mapping }))
    .length
}

export function formatBytes(bytes: number): string {
  if (bytes < 1_000_000) return `${Math.max(1, Math.round(bytes / 1000))} KB`
  return `${(bytes / 1_000_000).toFixed(1)} MB`
}

function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`
}

export function summaryLine(summary: Summary): string {
  const panels =
    summary.rows > 0
      ? `${count(summary.panels, "panel")} in ${count(summary.rows, "row")}`
      : count(summary.panels, "panel")
  return [
    formatBytes(summary.bytes),
    summary.schema_version > 0
      ? `Grafana schema ${summary.schema_version}`
      : "",
    panels,
    count(summary.variables, "variable"),
  ]
    .filter((part) => part !== "")
    .join(" · ")
}

export function isEditableTarget(target: EventTarget | null): boolean {
  if (target === null || !("tagName" in target)) return false
  const element = target as { tagName: string; isContentEditable?: boolean }
  return (
    element.isContentEditable === true ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName)
  )
}

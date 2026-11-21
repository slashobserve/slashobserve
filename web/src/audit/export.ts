import type { AuditWindowKey } from "@/api/keys"

export type ExportFormat = "ndjson" | "csv"

const formatNames: Record<ExportFormat, string> = {
  ndjson: "NDJSON",
  csv: "CSV",
}

export const exportRowLimit = 1_000_000

const exportPath = "/api/v1/audit/export"

export function auditExportUrl(
  window: AuditWindowKey,
  format: ExportFormat
): string {
  const params = new URLSearchParams()
  if (window.q !== "") params.set("q", window.q)
  params.set("since", window.since)
  params.set("until", window.until)
  params.set("format", format)
  return `${exportPath}?${params}`
}

export function exportLimitText(total: number): string | null {
  if (total <= exportRowLimit) return null
  const count = (n: number) => n.toLocaleString("en-US")
  return `${count(total)} events match; the export stops at ${count(exportRowLimit)}.`
}

export function downloadStartedText(format: ExportFormat): string {
  return `Download started: the audit log as ${formatNames[format]}.`
}

export function startDownload(url: string) {
  const link = document.createElement("a")
  link.href = url
  link.download = ""
  document.body.append(link)
  link.click()
  link.remove()
}

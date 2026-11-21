import type { AuditWindowKey } from "@/api/keys"

export type ExportFormat = "ndjson" | "csv"

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

export function startDownload(url: string) {
  const link = document.createElement("a")
  link.href = url
  link.download = ""
  document.body.append(link)
  link.click()
  link.remove()
}

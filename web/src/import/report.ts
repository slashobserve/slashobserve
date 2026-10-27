import type { StatusTone } from "@/components/status-label"
import type { components } from "@/api/schema.gen"

import type { ImportSourceDatasource } from "./errors"

export type ImportReport = components["schemas"]["ImportReport"]
type Status = components["schemas"]["ImportPanelLine"]["status"]

export type ReportFilter = "attention" | "partial" | "placeholder" | "all"

export type ReportLine = {
  key: string
  kind: "panel" | "note"
  title: string
  subtitle: string | null
  status: Status
  reasons: string[]
  paths: string
}

export const reportFilters: { id: ReportFilter; label: string }[] = [
  { id: "attention", label: "Needs a look" },
  { id: "partial", label: "Partial" },
  { id: "placeholder", label: "Placeholders" },
  { id: "all", label: "All panels" },
]

export function filterCounts(
  report: ImportReport
): Record<ReportFilter, number> {
  const { total, imported, partial, placeholder } = report.counts
  return {
    attention: total - imported,
    partial,
    placeholder,
    all: total,
  }
}

export function initialFilter(report: ImportReport): ReportFilter {
  return filterCounts(report).attention > 0 ? "attention" : "all"
}

export function statusLabel(status: Status): {
  tone: StatusTone
  text: string
} {
  switch (status) {
    case "imported":
      return { tone: "ok", text: "Imported" }
    case "partial":
      return { tone: "warning", text: "Partial" }
    case "placeholder":
      return { tone: "warning", text: "Placeholder" }
    case "unverified":
      return { tone: "critical", text: "Unverified" }
  }
}

function panelLines(report: ImportReport, filter: ReportFilter): ReportLine[] {
  return report.panels
    .filter((panel) => {
      switch (filter) {
        case "attention":
          return panel.status !== "imported"
        case "partial":
          return panel.status === "partial"
        case "placeholder":
          return panel.status === "placeholder"
        case "all":
          return true
      }
    })
    .map((panel) => {
      const type =
        panel.status === "placeholder" ? panel.source_type : panel.type
      return {
        key: `panel:${panel.panel_id}`,
        kind: "panel",
        title: panel.title,
        subtitle: panel.row === null ? type : `${type} · ${panel.row}`,
        status: panel.status,
        reasons: panel.reasons.length > 0 ? panel.reasons : ["Imported as is."],
        paths: panel.paths.join(", "),
      }
    })
}

function noteLines(report: ImportReport, filter: ReportFilter): ReportLine[] {
  if (filter !== "attention" && filter !== "all") return []
  return report.notes.map((note, index) => ({
    key: `note:${index}`,
    kind: "note",
    title: note.subject,
    subtitle: null,
    status: note.status,
    reasons: [note.reason],
    paths: note.paths.join(", "),
  }))
}

export function filterLines(
  report: ImportReport,
  filter: ReportFilter
): ReportLine[] {
  return [...panelLines(report, filter), ...noteLines(report, filter)]
}

export function visibleDatasources(
  datasources: ImportSourceDatasource[]
): ImportSourceDatasource[] {
  return datasources.filter(
    (datasource) => datasource.kind !== null || datasource.panels > 0
  )
}

export function datasourceKindLabel(kind: string): string {
  if (kind === "clickhouse") return "ClickHouse"
  if (kind === "prometheus") return "Prometheus"
  return kind
}

export function blockingReason(
  datasources: ImportSourceDatasource[]
): string | null {
  const blocking = visibleDatasources(datasources).find(
    (datasource) => datasource.kind !== null && datasource.match === null
  )
  if (!blocking) return null
  return blocking.candidates.length === 0
    ? "Some datasources are not configured."
    : `Choose a datasource for ${blocking.key}.`
}

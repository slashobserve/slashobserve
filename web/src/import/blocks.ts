import type { StatusTone } from "@/components/status-label"
import type { components } from "@/api/schema.gen"

import { statusLabel, type ImportReport } from "./report"

export type ImportBlock = components["schemas"]["ImportBlock"]
export type ImportOptionLine = components["schemas"]["ImportOptionLine"]
export type Outcome = ImportOptionLine["outcome"]

type Label = { tone: StatusTone; text: string }

export type BlockTally = {
  mapped: number
  excluded: number
  reported: number
  implicit: number
  omitted: number
}

export function outcomeLabel(outcome: Outcome): Label {
  switch (outcome) {
    case "mapped":
      return { tone: "ok", text: "Mapped" }
    case "excluded":
      return { tone: "none", text: "Excluded by decision" }
    case "reported":
      return { tone: "warning", text: "Not imported" }
    case "default":
      return { tone: "none", text: "Grafana default" }
    case "no_effect":
      return { tone: "none", text: "No visible effect" }
    case "bookkeeping":
      return { tone: "none", text: "Editor bookkeeping" }
  }
}

export function isImplicit(outcome: Outcome): boolean {
  return (
    outcome === "default" ||
    outcome === "no_effect" ||
    outcome === "bookkeeping"
  )
}

export function splitOptions(block: ImportBlock): {
  main: ImportOptionLine[]
  implicit: ImportOptionLine[]
} {
  return {
    main: block.options.filter((line) => !isImplicit(line.outcome)),
    implicit: block.options.filter((line) => isImplicit(line.outcome)),
  }
}

export function blockHeading(block: ImportBlock): {
  title: string
  subtitle: string | null
} {
  const subtitle = block.source_type === "" ? null : block.source_type
  switch (block.kind) {
    case "dashboard":
      return { title: "Dashboard settings", subtitle }
    case "variable":
      return { title: `Variable ${block.title}`, subtitle }
    case "row":
      return { title: `Row ${block.title}`, subtitle }
    case "panel":
      return {
        title: block.title === "" ? "Untitled panel" : block.title,
        subtitle,
      }
    case "query":
      return { title: `Query ${block.title}`, subtitle }
    case "transformation":
      return { title: `Transformation ${block.title}`, subtitle }
    case "field_config":
      return { title: "Field config", subtitle }
    case "override":
      return { title: `Override ${block.title}`, subtitle }
  }
}

export function blockTally(block: ImportBlock): BlockTally {
  const tally: BlockTally = {
    mapped: 0,
    excluded: block.outcome === "excluded" ? 1 : 0,
    reported: block.outcome === "reported" ? 1 : 0,
    implicit: block.implicit,
    omitted: block.omitted,
  }
  for (const line of block.options) {
    if (line.outcome === "mapped") tally.mapped++
    else if (line.outcome === "excluded") tally.excluded++
    else if (line.outcome === "reported") tally.reported++
  }
  for (const child of block.blocks) {
    const part = blockTally(child)
    tally.mapped += part.mapped
    tally.excluded += part.excluded
    tally.reported += part.reported
    tally.implicit += part.implicit
    tally.omitted += part.omitted
  }
  return tally
}

export function tallyText(tally: BlockTally): string {
  const parts: [number, string][] = [
    [tally.mapped, "mapped"],
    [tally.excluded, "excluded"],
    [tally.reported, "not imported"],
    [tally.implicit, "need no setting"],
  ]
  const shown = parts
    .filter(([count]) => count !== 0)
    .map(([count, text]) => `${count} ${text}`)
  return shown.length === 0 ? "No options" : shown.join(" · ")
}

const unverified: Label = { tone: "critical", text: "Unverified" }

export function blockStatus(
  block: ImportBlock,
  report: ImportReport
): Label | null {
  switch (block.kind) {
    case "panel":
    case "row": {
      const line = report.panels.find(
        (panel) => panel.panel_id === block.panel_id
      )
      return line ? statusLabel(line.status) : null
    }
    case "variable":
      return report.notes.some(
        (note) =>
          note.status === "unverified" && note.paths.includes(block.path)
      )
        ? unverified
        : null
    case "dashboard":
      return report.notes.some(
        (note) => note.subject === "dashboard" && note.status === "unverified"
      )
        ? unverified
        : null
    default:
      return null
  }
}

export function specText(line: ImportOptionLine): string {
  switch (line.outcome) {
    case "mapped":
      return line.spec.length === 0
        ? "Spec field not listed"
        : `→ ${line.spec.join(", ")}`
    case "excluded":
    case "reported":
      return line.reason ?? ""
    default:
      return ""
  }
}

export function implicitText(block: ImportBlock): string | null {
  if (block.implicit === 0) return null
  return block.implicit === 1
    ? "1 option needs no setting"
    : `${block.implicit} options need no setting`
}

export function omittedText(block: ImportBlock): string | null {
  if (block.omitted === 0) return null
  return block.omitted === 1
    ? "1 more option is not listed"
    : `${block.omitted} more options are not listed`
}

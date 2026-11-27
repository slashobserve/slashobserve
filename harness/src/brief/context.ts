import type { RunBrief } from "../protocol"
import {
  byteLength,
  fields,
  formatRFC3339,
  goJSON,
  goQuote,
  unixSeconds,
} from "./gofmt"
import { buildSection, seedNote, type Seed } from "./seed"

export type GridPos = { x: number; y: number; w: number; h: number }

export type Panel = {
  id: string
  type: string
  title?: string
  gridPos: GridPos
  panels?: Panel[]
}

export type Dashboard = {
  title: string
  time: unknown
  refresh?: string
  tags?: string[]
  links?: unknown[]
  variables?: { name: string }[]
  panels: Panel[]
}

type Issue = { code: string; message: string }

export type Unverified = {
  panels?: { panel_id: string; issues?: Issue[] | null }[] | null
  variables?: { name: string; issues?: Issue[] | null }[] | null
}

export type Attention = {
  panel_id: string
  title: string
  status: string
  reason: string
}

export type Revision = {
  dashboardID: string
  number: number
  spec: Dashboard
  unverified: Unverified
  conversion: Attention[]
}

export type Datasource = RunBrief["datasources"][number]

type Entry = { panel: Panel; row: Panel | null }

function allPanels(d: Dashboard): Entry[] {
  const entries: Entry[] = []
  for (const p of d.panels ?? []) {
    entries.push({ panel: p, row: null })
    for (const child of p.panels ?? []) {
      entries.push({ panel: child, row: p })
    }
  }
  return entries
}

function findPanel(d: Dashboard, id: string): Entry | undefined {
  return allPanels(d).find((e) => e.panel.id === id)
}

function panelTitle(d: Dashboard, id: string): string | undefined {
  const e = findPanel(d, id)
  return e === undefined ? undefined : (e.panel.title ?? "")
}

export function writeDatasources(statuses: readonly Datasource[]): string {
  let out = "Configured datasources:\n"
  const sorted = [...statuses].sort((a, b) =>
    Buffer.compare(Buffer.from(a.uid), Buffer.from(b.uid))
  )
  for (const s of sorted) {
    const kind = s.spans ? s.type + ", spans: " + s.spans : s.type
    out += `- ${s.uid} (${kind}): ${s.healthy ? "healthy" : "unhealthy"}\n`
  }
  return out
}

const newDashboardLimits =
  " Without a folder_id the dashboard is saved in the requester's My Dashboards. If the request names a folder, find it with list_folders and pass its id as folder_id on your first save; use only a folder with can_create true, and when none fits, build it in My Dashboards and say in the last save's summary that it can be moved. This run cannot create alerts, and a dashboard that already exists is changed from its own chat. To change one, find it with list_dashboards by the id or exact title in the request, reading at most 3 pages with next_cursor: when exactly one dashboard matches, answer with save_finding that says the change is asked for on that dashboard's own chat, and pass its id as ask_on_dashboard_id; when none or several match, answer with save_finding that lists up to 5 candidate titles and pass no ask_on_dashboard_id. For an alert, answer with save_finding that says so, and that a dashboard with a threshold line can be asked for instead."

const outlineThresholdBytes = 16 << 10
const maxOutlineTargets = 20
const maxListLines = 60
const maxListBytes = 8 << 10

export type BuildContextInput = {
  statuses: readonly Datasource[]
  mode: string
  source: string
  panelID: string
  prompt: string
  current: Revision | null
  hasBuild: boolean
  seed: Seed
}

export function buildContext(input: BuildContextInput): string {
  const { mode, current, seed } = input
  let out = writeDatasources(input.statuses)
  if (mode === "create" && !input.hasBuild) {
    out +=
      "\nCreate a new dashboard for the request, or answer it with save_finding if it only asks a question."
    return out + newDashboardLimits + seedNote(seed)
  }
  if (current === null) {
    throw new Error("the brief has no dashboard to work on")
  }
  const body = goJSON(current.spec)
  out += intro(mode, current, input.hasBuild)
  if (byteLength(body) > outlineThresholdBytes) {
    out +=
      " The Spec is too large to send whole, so this is its outline: read the full JSON of other panels with get_panels.\n"
    out += outline(
      current.spec,
      outlineTargets(current.spec, input.panelID, input.prompt)
    )
  } else {
    out += "\n```json\n" + body + "\n```"
  }
  if (mode === "create") {
    return out + buildSection(seed)
  }
  out += unverifiedLines(current) + conversionLines(current)
  if (mode === "suggest") {
    return out + suggestRequest(input.panelID, current)
  }
  out += panelRequest(input.source, input.panelID, current)
  return out + (input.hasBuild ? buildSection(seed) : seedNote(seed))
}

function intro(mode: string, current: Revision, hasBuild: boolean): string {
  if (mode === "suggest") {
    return `\nYou are proposing a change to dashboard ${current.dashboardID}, version ${current.number}.`
  }
  if (mode === "create") {
    return "\nCreate a new dashboard for the request. This is your build of the new dashboard, not saved as a version yet."
  }
  if (hasBuild) {
    return `\nYou are revising dashboard ${current.dashboardID}, currently version ${current.number}. This is your build of the revision, not saved as a version yet.`
  }
  return `\nYou are revising dashboard ${current.dashboardID}, currently version ${current.number}. Change it for the request, or answer it with save_finding if it only asks a question.`
}

function suggestRequest(panelID: string, current: Revision): string {
  const title = panelTitle(current.spec, panelID) || panelID
  let out = `\nPropose a change to panel ${panelID} titled ${goQuote(title)} for the request. Call update_panels with only that panel, with id ${panelID}, in upsert; everything else stays as in version ${current.number}, and nothing is saved.`
  const names = (current.spec.variables ?? []).map((v) => v.name)
  if (names.length > 0) {
    out += ` The dashboard has the variables ${names.join(", ")}; when your change stops the panel using one, say so in summary.`
  }
  return out
}

function panelRequest(
  source: string,
  panelID: string,
  current: Revision
): string {
  if (panelID === "") {
    return ""
  }
  const fromComment = source === "comment"
  const title = panelTitle(current.spec, panelID)
  if (title !== undefined) {
    return fromComment
      ? `\nThe request is a comment on panel ${panelID} titled ${goQuote(title)}.`
      : `\nThe request is about panel ${panelID} titled ${goQuote(title)}.`
  }
  return fromComment
    ? `\nPanel ${panelID} from the comment is not in the latest version.`
    : `\nPanel ${panelID} from the request is not in the latest version.`
}

function firstIssue(issues: readonly Issue[] | null | undefined): string {
  const first = issues?.[0]
  return first === undefined
    ? ""
    : `${first.code}: ${fields(first.message).join(" ")}`
}

function unverifiedLines(current: Revision): string {
  const panels = current.unverified.panels ?? []
  const variables = current.unverified.variables ?? []
  if (panels.length === 0 && variables.length === 0) {
    return ""
  }
  const lines = [
    ...panels.map(
      (p) =>
        `- panel ${p.panel_id} ${goQuote(panelTitle(current.spec, p.panel_id) ?? "")}: ${firstIssue(p.issues)}`
    ),
    ...variables.map((v) => `- variable ${v.name}: ${firstIssue(v.issues)}`),
  ]
  return (
    `\nUnverified in version ${current.number}: their queries failed when it was saved. A save keeps them unverified while their queries and the variables those use stay unchanged; a changed or new query must return data.` +
    capped(lines) +
    "\nKeep unverified panels unless the request asks to delete them."
  )
}

function conversionLines(current: Revision): string {
  if (current.conversion.length === 0) {
    return ""
  }
  const lines = current.conversion.map(
    (a) =>
      `- ${a.status} ${a.panel_id} ${goQuote(a.title)}: ${fields(a.reason).join(" ")}`
  )
  return (
    `\nVersion ${current.number} is a Grafana import. The importer converted these panels only partly or kept them as placeholders:` +
    capped(lines)
  )
}

function capped(lines: readonly string[]): string {
  let out = ""
  let size = 0
  for (const [i, line] of lines.entries()) {
    const n = byteLength(line)
    if (i === maxListLines || size + n > maxListBytes) {
      out += `\n- ... and ${lines.length - i} more`
      break
    }
    size += n
    out += "\n" + line
  }
  return out
}

const panelIDRunes = /[^a-zA-Z0-9_-]+/

function outlineTargets(
  d: Dashboard,
  panelID: string,
  prompt: string
): string[] {
  const mentioned = new Set(prompt.split(panelIDRunes).filter((t) => t !== ""))
  const targets: string[] = []
  if (findPanel(d, panelID) !== undefined) {
    targets.push(panelID)
  }
  for (const e of allPanels(d)) {
    if (targets.length === maxOutlineTargets) {
      break
    }
    if (mentioned.has(e.panel.id) && !targets.includes(e.panel.id)) {
      targets.push(e.panel.id)
    }
  }
  return targets
}

function outline(d: Dashboard, targets: readonly string[]): string {
  let out = `Title: ${goJSON(d.title)}\n`
  out += `Time: ${goJSON(d.time)}\n`
  out += `Refresh: ${goJSON(d.refresh ?? "")}\n`
  out += `Tags: ${goJSON(d.tags ?? [])}\n`
  out += `Links: ${goJSON(d.links ?? [])}\n`
  out += `Variables: ${goJSON(d.variables ?? [])}\n`
  out += "Panels (id type title, row, position as x y w h):"
  for (const { panel: p, row } of allPanels(d)) {
    out += `\n- ${p.id} ${p.type} ${goQuote(p.title ?? "")}`
    if (row !== null) {
      out += ` row=${row.id}`
    }
    out += ` x${p.gridPos.x} y${p.gridPos.y} w${p.gridPos.w} h${p.gridPos.h}`
  }
  if (targets.length === 0) {
    return out
  }
  const full: Panel[] = []
  for (const id of targets) {
    const e = findPanel(d, id)
    if (e !== undefined) {
      const { panels: _children, ...alone } = e.panel
      full.push(alone)
    }
  }
  return (
    out +
    "\nPanels in full, rows without their children:\n```json\n" +
    goJSON(full) +
    "\n```"
  )
}

export type InvestigateContextInput = {
  statuses: readonly Datasource[]
  from: string | null
  to: string | null
  panelID: string
  prompt: string
  current: Revision | null
  seed: Seed
  triage: string
}

export function investigateContext(input: InvestigateContextInput): string {
  let out = writeDatasources(input.statuses)
  out += "\nInvestigate the question and save one finding."
  if (input.from && input.to) {
    out += `\nWindow: ${formatRFC3339(unixSeconds(input.from))} to ${formatRFC3339(unixSeconds(input.to))}`
  } else {
    out += "\nWindow: the last hour (a default the question may need to exceed)"
  }
  const current = input.current
  if (current !== null) {
    out += `\nThe question is about dashboard ${current.dashboardID}, version ${current.number}.\n`
    out += outline(
      current.spec,
      outlineTargets(current.spec, input.panelID, input.prompt)
    )
    const title = panelTitle(current.spec, input.panelID)
    if (title !== undefined) {
      out += `\nThe question is about panel ${input.panelID} titled ${goQuote(title)}, shown in full above.`
    }
  }
  out += seedNote(input.seed)
  if (input.triage !== "") {
    out += "\n\n" + input.triage
  }
  return out
}

import { variablePrefix, type DashboardSearchParams } from "@/dashboards/search"
import { dashboardPath } from "@/lib/links"
import { stringifySearch } from "@/lib/search"
import {
  isDataPanel,
  panelTitle,
  type Dashboard,
  type LayoutPanel,
  type Panel,
} from "@/spec"

export type ShareOptions = { time: boolean; variables: boolean }

const timeKeys = ["from", "to"]

export function offersVariables(spec: Pick<Dashboard, "variables">): boolean {
  return (spec.variables ?? []).length > 0
}

type LinkQuery = Record<string, string | string[] | number>

function linkSearch(
  search: DashboardSearchParams,
  options: ShareOptions
): LinkQuery {
  const out: LinkQuery = {}
  const keep = (key: string) => {
    const value = search[key]
    if (value !== undefined) out[key] = value
  }
  if (options.time) timeKeys.forEach(keep)
  if (options.variables) {
    Object.keys(search)
      .filter((key) => key.startsWith(variablePrefix))
      .forEach(keep)
  }
  return out
}

type SharedDashboard = { id: string; title: string }

function dashboardUrl(
  origin: string,
  dashboard: SharedDashboard,
  query: LinkQuery
): string {
  return `${origin}${dashboardPath(dashboard.id, dashboard.title)}${stringifySearch(query)}`
}

export function dashboardShareUrl(
  origin: string,
  dashboard: SharedDashboard,
  search: DashboardSearchParams,
  options: ShareOptions
): string {
  return dashboardUrl(origin, dashboard, linkSearch(search, options))
}

export type PanelChoice = { id: string; title: string }

type Positioned = { gridPos: { x: number; y: number } }

function inLayoutOrder<T extends Positioned>(panels: readonly T[]): T[] {
  return [...panels].sort(
    (a, b) => a.gridPos.y - b.gridPos.y || a.gridPos.x - b.gridPos.x
  )
}

function isLinkable(panel: Panel): panel is LayoutPanel {
  return panel.type === "text" || isDataPanel(panel)
}

export function panelChoices(spec: Pick<Dashboard, "panels">): PanelChoice[] {
  return inLayoutOrder(spec.panels)
    .flatMap((panel) =>
      panel.type === "row" ? inLayoutOrder(panel.panels ?? []) : [panel]
    )
    .filter(isLinkable)
    .map((panel) => ({ id: panel.id, title: panelTitle(panel) }))
}

export function sharedPanelId(
  choices: readonly PanelChoice[],
  chosen: string | undefined,
  viewed: string | undefined
): string | undefined {
  const has = (id: string | undefined) =>
    id !== undefined && choices.some((choice) => choice.id === id)
  if (has(chosen)) return chosen
  if (has(viewed)) return viewed
  return choices[0]?.id
}

export function panelShareUrl(
  origin: string,
  dashboard: SharedDashboard,
  panelId: string,
  search: DashboardSearchParams,
  options: ShareOptions
): string {
  return dashboardUrl(origin, dashboard, {
    ...linkSearch(search, options),
    panel: panelId,
  })
}

export function linkHint(personal: boolean): string {
  return personal
    ? "Only you can open this link until you publish the dashboard."
    : 'Links need sign-in. Someone without access to the folder sees "Dashboard not found".'
}

import { variablePrefix, type DashboardSearchParams } from "@/dashboards/search"
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

function dashboardUrl(
  origin: string,
  dashboardId: string,
  query: LinkQuery
): string {
  return `${origin}/dashboards/${encodeURIComponent(dashboardId)}${stringifySearch(query)}`
}

export function dashboardShareUrl(
  origin: string,
  dashboardId: string,
  search: DashboardSearchParams,
  options: ShareOptions
): string {
  return dashboardUrl(origin, dashboardId, linkSearch(search, options))
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

export function panelShareUrl(
  origin: string,
  dashboardId: string,
  panelId: string,
  search: DashboardSearchParams,
  options: ShareOptions
): string {
  return dashboardUrl(origin, dashboardId, {
    ...linkSearch(search, options),
    panel: panelId,
  })
}

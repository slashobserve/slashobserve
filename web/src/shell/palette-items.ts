import type { components } from "@/api/schema.gen"
import { dashboardPath, observationPath } from "@/lib/links"
import { roleAtLeast, type Role } from "@/lib/roles"

import { visibleNavItems, type NavPath } from "./nav"

export type SearchResult = components["schemas"]["SearchResult"]
export type MatchSegment = components["schemas"]["SearchSnippetSegment"]

export type PagePath = NavPath | "/keys"
type PageEntry = {
  kind: "page"
  id: string
  label: string
  to: PagePath
}
export type ResultEntry = {
  kind: SearchResult["kind"]
  id: string
  label: string
  secondary: MatchSegment[]
  path: string
}
type ActionId =
  | "action:new-observation"
  | "action:ask"
  | "action:import"
  | "action:sign-out"
  | "action:theme"
type ActionEntry = {
  kind: "action"
  id: ActionId
  label: string
}
export type PaletteEntry = PageEntry | ResultEntry | ActionEntry
export type PaletteGroup = {
  value:
    | "Pages"
    | "Dashboards"
    | "Observations"
    | "Comments"
    | "Annotations"
    | "Saved searches"
    | "Actions"
    | "Account"
  items: PaletteEntry[]
}

const keysEntry: PageEntry = {
  kind: "page",
  id: "page:/keys",
  label: "API keys",
  to: "/keys",
}

function pageEntries(role: Role): PageEntry[] {
  const entries: PageEntry[] = visibleNavItems(role).map((item) => ({
    kind: "page",
    id: `page:${item.to}`,
    label: item.label,
    to: item.to,
  }))
  if (!roleAtLeast(role, "admin")) return entries
  const before = entries.findIndex((entry) => entry.to === "/people")
  return entries.toSpliced(before, 0, keysEntry)
}

const newActionEntries: ActionEntry[] = [
  { kind: "action", id: "action:new-observation", label: "New observation" },
  { kind: "action", id: "action:ask", label: "Ask Observe Agent" },
  { kind: "action", id: "action:import", label: "Import a dashboard" },
]

const themeEntry: ActionEntry = {
  kind: "action",
  id: "action:theme",
  label: "Switch theme",
}

const signOutEntry: ActionEntry = {
  kind: "action",
  id: "action:sign-out",
  label: "Sign out",
}

const dashboardLimit = 20
const dashboardLimitWithoutQuery = 8

export const minSearchLength = 2

const serverGroups = [
  { value: "Observations", kind: "observation" },
  { value: "Comments", kind: "comment" },
  { value: "Annotations", kind: "annotation" },
  { value: "Saved searches", kind: "saved_search" },
] as const

function pathOf(address: string): string {
  const url = new URL(address, "http://localhost")
  return url.pathname + url.search
}

function folderText(result: SearchResult): MatchSegment[] {
  return result.folder === null
    ? []
    : [{ text: result.folder.title, match: false }]
}

function serverEntry(result: SearchResult, address: string): ResultEntry {
  const path = pathOf(address)
  switch (result.kind) {
    case "dashboard":
      return {
        kind: "dashboard",
        id: result.id,
        label: result.title,
        secondary: folderText(result),
        path,
      }
    case "saved_search":
      return {
        kind: "saved_search",
        id: `saved_search:${result.id}`,
        label: result.title,
        secondary: folderText(result),
        path,
      }
    case "observation":
      return {
        kind: "observation",
        id: `observation:${result.id}`,
        label: result.title,
        secondary: result.snippet,
        path: observationPath(result.id),
      }
    case "comment":
      return {
        kind: "comment",
        id: `comment:${result.id}`,
        label: `Comment on ${result.title}`,
        secondary: result.snippet,
        path,
      }
    default:
      return {
        kind: result.kind,
        id: `${result.kind}:${result.id}`,
        label: result.title,
        secondary: result.snippet,
        path,
      }
  }
}

export function paletteGroups(
  query: string,
  dashboards: { id: string; title: string }[],
  results: SearchResult[],
  role: Role
): PaletteGroup[] {
  const needle = query.trim().toLowerCase()
  const searching = needle.length >= minSearchLength
  const matches = (label: string) => label.toLowerCase().includes(needle)
  const localDashboards: ResultEntry[] = dashboards
    .filter((dashboard) => matches(dashboard.title))
    .map((dashboard) => ({
      kind: "dashboard",
      id: dashboard.id,
      label: dashboard.title,
      secondary: [],
      path: dashboardPath(dashboard.id, dashboard.title),
    }))
  const shown = new Set(localDashboards.map((entry) => entry.id))
  const found = searching
    ? results.flatMap((result) =>
        result.url === null ? [] : [serverEntry(result, result.url)]
      )
    : []
  const dashboardEntries = [
    ...localDashboards,
    ...found.filter(
      (entry) => entry.kind === "dashboard" && !shown.has(entry.id)
    ),
  ].slice(0, needle === "" ? dashboardLimitWithoutQuery : dashboardLimit)
  const groups: PaletteGroup[] = [
    {
      value: "Pages",
      items: pageEntries(role).filter((e) => matches(e.label)),
    },
    { value: "Dashboards", items: dashboardEntries },
    ...serverGroups.map(({ value, kind }) => ({
      value,
      items: found.filter((entry) => entry.kind === kind),
    })),
    {
      value: "Actions",
      items: newActionEntries.filter((e) => matches(e.label)),
    },
    {
      value: "Account",
      items: [themeEntry, signOutEntry].filter((e) => matches(e.label)),
    },
  ]
  return groups.filter((group) => group.items.length > 0)
}

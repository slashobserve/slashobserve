import type { components } from "@/api/schema.gen"
import {
  exploreText,
  sourceLanguage,
  type ExploreKind,
} from "@/datasources/sources"
import {
  dashboardPath,
  logsPath,
  metricsPath,
  observationPath,
  tracesPath,
} from "@/lib/links"
import type { Role } from "@/lib/roles"

import { plainSegments } from "@/search/plain-text"

import { visibleNavItems, type NavPath } from "./nav"

export type SearchResult = components["schemas"]["SearchResult"]
export type MatchSegment = components["schemas"]["SearchSnippetSegment"]

export type PagePath = NavPath
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
export type SourceEntry = {
  kind: "source"
  id: string
  label: string
  secondary: string
  path: string
  explore: ExploreKind
}
export type PaletteSource = {
  explore: ExploreKind
  uid: string
  name: string
  type: string
}
type ActionId =
  | "action:inbox"
  | "action:new-observation"
  | "action:ask"
  | "action:import"
  | "action:sign-out"
  | "action:theme"
  | "action:docs"
  | "action:shortcuts"
  | "action:search-all"
type ActionEntry = {
  kind: "action"
  id: ActionId
  label: string
}
export type PaletteEntry = PageEntry | ResultEntry | SourceEntry | ActionEntry
export type PaletteGroup = {
  value:
    | "Pages"
    | "Dashboards"
    | "Sources"
    | "Recent observations"
    | "Observations"
    | "Comments"
    | "Annotations"
    | "Saved searches"
    | "Actions"
    | "Search"
    | "Help"
    | "Account"
  items: PaletteEntry[]
}

function pageEntries(role: Role): PageEntry[] {
  return visibleNavItems(role).map((item) => ({
    kind: "page",
    id: `page:${item.to}`,
    label: item.label,
    to: item.to,
  }))
}

const inboxEntry: ActionEntry = {
  kind: "action",
  id: "action:inbox",
  label: "Inbox",
}

const newActionEntries: ActionEntry[] = [
  { kind: "action", id: "action:new-observation", label: "New observation" },
  { kind: "action", id: "action:ask", label: "Ask Observe Agent" },
  { kind: "action", id: "action:import", label: "Import a dashboard" },
]

const docsEntry: ActionEntry = {
  kind: "action",
  id: "action:docs",
  label: "Help and docs",
}

const shortcutsEntry: ActionEntry = {
  kind: "action",
  id: "action:shortcuts",
  label: "Keyboard shortcuts",
}

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
const sourceLimit = 8

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

export function serverEntry(
  result: SearchResult,
  address: string
): ResultEntry {
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
        secondary: plainSegments(result.snippet),
        path: observationPath(result.id),
      }
    case "comment":
      return {
        kind: "comment",
        id: `comment:${result.id}`,
        label: `Comment on ${result.title}`,
        secondary: plainSegments(result.snippet),
        path,
      }
    default:
      return {
        kind: result.kind,
        id: `${result.kind}:${result.id}`,
        label: result.title,
        secondary: plainSegments(result.snippet),
        path,
      }
  }
}

function sourceEntries(
  sources: readonly PaletteSource[],
  needle: string
): SourceEntry[] {
  if (needle === "") return []
  return sources
    .filter(({ explore, name, type }) =>
      [name, explore, type].some((text) => text.toLowerCase().includes(needle))
    )
    .slice(0, sourceLimit)
    .map(({ explore, uid, name, type }) => ({
      kind: "source",
      id: `source:${explore}:${uid}`,
      label: name,
      secondary: `${exploreText[explore].page}, ${sourceLanguage(explore, type)}`,
      path: { logs: logsPath, traces: tracesPath, metrics: metricsPath }[
        explore
      ](uid),
      explore,
    }))
}

const recentObservationLimit = 5

export function paletteGroups(
  query: string,
  dashboards: { id: string; title: string }[],
  results: SearchResult[],
  role: Role,
  sources: readonly PaletteSource[],
  recentObservations: readonly { id: string; title: string }[] = []
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
      items: [...pageEntries(role), inboxEntry].filter((e) => matches(e.label)),
    },
    { value: "Dashboards", items: dashboardEntries },
    { value: "Sources", items: sourceEntries(sources, needle) },
    {
      value: "Recent observations",
      items:
        needle === ""
          ? recentObservations
              .slice(0, recentObservationLimit)
              .map((observation): ResultEntry => ({
                kind: "observation",
                id: `recent-observation:${observation.id}`,
                label: observation.title,
                secondary: [],
                path: observationPath(observation.id),
              }))
          : [],
    },
    ...serverGroups.map(({ value, kind }) => ({
      value,
      items: found.filter((entry) => entry.kind === kind),
    })),
    {
      value: "Actions",
      items: newActionEntries.filter((e) => matches(e.label)),
    },
    {
      value: "Help",
      items: [docsEntry, shortcutsEntry].filter((e) => matches(e.label)),
    },
    {
      value: "Account",
      items: [themeEntry, signOutEntry].filter((e) => matches(e.label)),
    },
    {
      value: "Search",
      items: searching
        ? [
            {
              kind: "action",
              id: "action:search-all",
              label: `See all results for "${query.trim()}"`,
            },
          ]
        : [],
    },
  ]
  return groups.filter((group) => group.items.length > 0)
}

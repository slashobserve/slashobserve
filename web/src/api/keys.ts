export type Selections = Record<string, string[]>

export type PanelDataParams = {
  from?: string
  to?: string
  selections: Selections
  maxDataPoints?: number
}

export type VariableValuesParams = {
  from?: string
  to?: string
  selections: Selections
}

export type DashboardListFilters = {
  q?: string
  madeBy?: string
  datasource?: string
  tags?: string[]
  folder?: string
}

export const dashboardKeys = {
  all: ["dashboards"] as const,
  one: (id: string) => ["dashboards", id] as const,
  list: () => ["dashboards", "list"] as const,
  filtered: (filters: DashboardListFilters, page: number) =>
    ["dashboards", "list", filters, page] as const,
  titles: () => ["dashboards", "titles"] as const,
  tags: () => ["dashboards", "tags"] as const,
  tagged: (tags: string[]) => ["dashboards", "list", "tagged", tags] as const,
  inFolder: (folderId: string) =>
    ["dashboards", "list", "folder", folderId] as const,
  latest: (id: string) => ["dashboards", id, "latest"] as const,
  version: (id: string, version: number) =>
    ["dashboards", id, "versions", version] as const,
  panels: (id: string, version: number) =>
    ["dashboards", id, "versions", version, "panels"] as const,
  panelData: (
    id: string,
    version: number,
    panelId: string,
    params: PanelDataParams
  ) =>
    ["dashboards", id, "versions", version, "panels", panelId, params] as const,
  variableValues: (
    id: string,
    version: number,
    name: string,
    params: VariableValuesParams
  ) =>
    ["dashboards", id, "versions", version, "variables", name, params] as const,
  history: (id: string) => ["dashboards", id, "history"] as const,
  diff: (id: string, from: number, to: number) =>
    ["dashboards", id, "diff", from, to] as const,
  comments: (id: string) => ["dashboards", id, "comments"] as const,
  revisions: (id: string) => ["dashboards", id, "revisions"] as const,
  revision: (id: string, revisionId: string) =>
    ["dashboards", id, "revision", revisionId] as const,
  saves: (id: string) => ["dashboards", id, "saves"] as const,
  deletes: (id: string) => ["dashboards", id, "deletes"] as const,
  posts: (id: string) => ["dashboards", id, "posts"] as const,
}

export const folderKeys = {
  all: ["folders"] as const,
  list: () => ["folders", "list"] as const,
  detail: (id: string) => ["folders", id] as const,
  permissions: (id: string) => ["folders", id, "permissions"] as const,
}

export type ChatListFilters = {
  source?: "observe" | "investigate"
  q?: string
  pinned?: boolean
  dashboardId?: string
}

export const observeKeys = {
  agent: () => ["agent"] as const,
  request: (id: string) => ["observe", id] as const,
  mine: (filters: ChatListFilters, limit: number) =>
    ["observe-mine", filters, limit] as const,
  mineAll: () => ["observe-mine"] as const,
  create: () => ["observe-create"] as const,
  build: (id: string, saves: number) => ["observe-build", id, saves] as const,
  buildPanelData: (
    id: string,
    panelId: string,
    fingerprint: string,
    params: PanelDataParams
  ) => ["observe-build", id, "panels", panelId, fingerprint, params] as const,
  buildVariableValues: (
    id: string,
    name: string,
    fingerprint: string,
    params: VariableValuesParams
  ) => ["observe-build", id, "variables", name, fingerprint, params] as const,
}

export const serviceAccountKeys = {
  all: ["service-accounts"] as const,
  list: () => ["service-accounts", "list"] as const,
  keys: (accountId: string) => ["service-accounts", accountId, "keys"] as const,
}

export type NameListParams = {
  search?: string
  metric?: string
  table?: string
}

export const datasourceKeys = {
  all: ["datasources"] as const,
  list: () => ["datasources", "list"] as const,
  metrics: (uid: string, search: string) =>
    ["datasources", uid, "metrics", search] as const,
  labels: (uid: string, params: NameListParams) =>
    ["datasources", uid, "labels", params] as const,
  labelValues: (uid: string, label: string, params: NameListParams) =>
    ["datasources", uid, "labels", label, "values", params] as const,
}

export type PreviewKeyParams = {
  panel: string
  from?: string
  to?: string
  selections: Selections
  maxDataPoints?: number
}

export const editorKeys = {
  preview: (
    dashboardId: string,
    version: number,
    panelId: string,
    params: PreviewKeyParams
  ) => ["editor-preview", dashboardId, version, panelId, params] as const,
  suggestion: (dashboardId: string, suggestionId: string) =>
    ["suggestions", dashboardId, suggestionId] as const,
}

export const promptKeys = {
  all: ["prompts"] as const,
  list: () => ["prompts", "list"] as const,
}

export const userKeys = {
  all: ["users"] as const,
  list: () => ["users", "list"] as const,
}

export const instanceKeys = {
  info: () => ["instance"] as const,
}

export const importKeys = {
  all: ["imports"] as const,
  preview: (sourceId: number, mapping: Record<string, string>) =>
    ["imports", "preview", sourceId, mapping] as const,
  list: (scope: "mine") => ["imports", "list", scope] as const,
  forDashboard: (id: string) => ["imports", "dashboard", id] as const,
}

export const grafanaKeys = {
  all: ["grafana"] as const,
  connections: () => ["grafana", "connections"] as const,
  folders: (connectionId: string) =>
    ["grafana", connectionId, "folders"] as const,
  datasources: (connectionId: string) =>
    ["grafana", connectionId, "datasources"] as const,
  sync: (connectionId: string, syncId: string) =>
    ["grafana", connectionId, "imports", syncId] as const,
}

export const repairKeys = {
  all: ["repairs"] as const,
  list: (folderId: string | null) =>
    ["repairs", "list", folderId ?? "all"] as const,
}

export const healthKeys = {
  all: ["health"] as const,
  list: (kind: string | null, folderId: string | null) =>
    ["health", "list", kind ?? "all", folderId ?? "all"] as const,
}

export const outboundDomainKeys = {
  list: () => ["outbound-domains"] as const,
}

export const assetKeys = {
  geojson: (url: string) => ["assets", "geojson", url] as const,
}

export const actionKeys = {
  run: (dashboardId: string) => ["actions", dashboardId] as const,
}

export const newsKeys = {
  feed: (url: string) => ["news", url] as const,
}

export type ExploreParams = {
  query: string
  kind: "range" | "instant"
  from: string
  to: string
  limit?: number
  maxDataPoints?: number
  pageSize?: number
  page?: number
  cursor?: string
}

export type TraceStatsParams = { query: string; from: string; to: string }
export type TraceCompareParams = {
  selected: string[]
  baseline: string[]
  from: string
  to: string
}
export type TraceLogsParams = {
  traceId: string
  spanId?: string
  service?: string
  from: string
  to: string
  limit?: number
}

export const exploreKeys = {
  all: ["explore"] as const,
  query: (uid: string, params: ExploreParams) =>
    ["explore", uid, "query", params] as const,
  tail: (uid: string, query: string, session: number) =>
    ["explore", uid, "tail", query, session] as const,
  context: (uid: string, anchor: string) =>
    ["explore", uid, "context", anchor] as const,
  trace: (uid: string, id: string, range: { from: string; to: string }) =>
    ["explore", uid, "trace", id, range] as const,
  traceStats: (uid: string, params: TraceStatsParams) =>
    ["explore", uid, "trace-stats", params] as const,
  traceCompare: (uid: string, params: TraceCompareParams) =>
    ["explore", uid, "trace-compare", params] as const,
  traceLogs: (uid: string, params: TraceLogsParams) =>
    ["explore", uid, "trace-logs", params] as const,
  targets: (search: string) => ["explore", "targets", search] as const,
}

export const findingKeys = {
  all: ["findings"] as const,
  one: (id: string) => ["findings", id] as const,
  forDashboard: (dashboardId: string) =>
    ["findings", "dashboard", dashboardId] as const,
  panelData: (
    id: string,
    panelId: string,
    params: { from?: string; to?: string; maxDataPoints?: number }
  ) => ["findings", id, "panels", panelId, params] as const,
}

export const annotationKeys = {
  all: ["annotations"] as const,
  range: (dashboardId: string, from: string, to: string) =>
    ["annotations", dashboardId, "range", from, to] as const,
  tags: (dashboardId: string) => ["annotations", dashboardId, "tags"] as const,
}

export type HomeSearch = { q?: string; tag: string[] }

export const starKeys = {
  all: ["stars"] as const,
  list: () => ["stars", "list"] as const,
}

export const homeKeys = {
  firstDashboards: () => ["dashboards", "list", "home-first"] as const,
  search: (search: HomeSearch) =>
    ["dashboards", "list", "home-search", search] as const,
}

export const publicKeys = {
  links: (dashboardId: string) => ["public-links", dashboardId] as const,
  dashboard: (token: string) => ["public", token] as const,
  panelData: (
    token: string,
    version: number,
    panelId: string,
    params: PanelDataParams
  ) =>
    ["public", token, "versions", version, "panels", panelId, params] as const,
  variableValues: (
    token: string,
    version: number,
    name: string,
    params: VariableValuesParams
  ) =>
    ["public", token, "versions", version, "variables", name, params] as const,
}

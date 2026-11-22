import type { components, paths } from "@/api/schema.gen"

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
  problem?: NonNullable<
    paths["/dashboards"]["get"]["parameters"]["query"]
  >["problem"]
  order?: "updated" | "name" | "views"
  excludeAudit?: boolean
}

export const dashboardKeys = {
  all: ["dashboards"] as const,
  one: (id: string) => ["dashboards", id] as const,
  list: () => ["dashboards", "list"] as const,
  filtered: (filters: DashboardListFilters, page: number) =>
    ["dashboards", "list", filters, page] as const,
  titles: () => ["dashboards", "titles"] as const,
  tags: () => ["dashboards", "tags"] as const,
  tagsFor: (excludeAudit: boolean) =>
    ["dashboards", "tags", excludeAudit] as const,
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
  viewers: (id: string) => ["dashboards", id, "viewers"] as const,
  revisions: (id: string) => ["dashboards", id, "revisions"] as const,
  revision: (id: string, revisionId: string) =>
    ["dashboards", id, "revision", revisionId] as const,
  saves: (id: string) => ["dashboards", id, "saves"] as const,
  deletes: (id: string) => ["dashboards", id, "deletes"] as const,
  posts: (id: string) => ["dashboards", id, "posts"] as const,
  views: (id: string) => ["dashboards", id, "views"] as const,
}

export const folderKeys = {
  all: ["folders"] as const,
  list: () => ["folders", "list"] as const,
  detail: (id: string) => ["folders", id] as const,
  permissions: (id: string) => ["folders", id, "permissions"] as const,
}

export type ChatListFilters = {
  q?: string
  pinned?: boolean
  dashboardId?: string
  observationId?: string
  chatId?: string
  scope?: "all"
  states?: components["schemas"]["RequestSummary"]["state"][]
  anchored?: true
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

export type ChatsFilters = {
  q?: string
  dashboardId?: string
  scope?: "all"
}

export const chatKeys = {
  list: (filters: ChatsFilters, limit: number) =>
    [...observeKeys.mineAll(), "chats", filters, limit] as const,
  detail: (id: string) => [...observeKeys.mineAll(), "chat", id] as const,
  lists: () => [...observeKeys.mineAll(), "chats"] as const,
  details: () => [...observeKeys.mineAll(), "chat"] as const,
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
  from?: string
  to?: string
}

export const datasourceKeys = {
  all: ["datasources"] as const,
  list: () => ["datasources", "list"] as const,
  config: (uid: string) => ["datasources", uid, "config"] as const,
  metrics: (uid: string, search: string) =>
    ["datasources", uid, "metrics", search] as const,
  labels: (uid: string, params: NameListParams) =>
    ["datasources", uid, "labels", params] as const,
  labelValues: (uid: string, label: string, params: NameListParams) =>
    ["datasources", uid, "labels", label, "values", params] as const,
  services: (uid: string, signal: "logs" | "traces") =>
    ["datasources", uid, "services", signal] as const,
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
  blockPreview: (
    observationId: string,
    blockId: string | null,
    version: number,
    params: PreviewKeyParams
  ) =>
    [
      "editor-preview",
      "observation",
      observationId,
      blockId,
      version,
      params,
    ] as const,
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

export const webhookKeys = {
  all: ["webhooks"] as const,
  list: () => ["webhooks", "list"] as const,
  deliveries: (id: string) => ["webhooks", id, "deliveries"] as const,
}

export const repairKeys = {
  all: ["repairs"] as const,
  list: (folderId: string | null) =>
    ["repairs", "list", folderId ?? "all"] as const,
}

export const healthKeys = {
  all: ["health"] as const,
  dashboards: (ids: readonly string[]) =>
    ["health", "dashboards", [...ids].sort()] as const,
  summary: (folderId: string | null) =>
    ["health", "summary", folderId ?? "all"] as const,
}

export const outboundDomainKeys = {
  list: () => ["outbound-domains"] as const,
}

export const grantKeys = {
  all: ["grants"] as const,
  list: (scope: "mine" | "all") => ["grants", scope] as const,
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
  sort?: string
  asWritten?: boolean
}

export type ContextParams = {
  query: string
  anchor: string
  before: number
  after: number
  window_ms: number
}

export type LogFacetsParams = {
  query: string
  from: string
  to: string
  limit: number
  fields?: string[]
}

export type LogPatternsParams = {
  query: string
  from: string
  to: string
  limit: number
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
  context: (uid: string, params: ContextParams) =>
    ["explore", uid, "context", params] as const,
  trace: (uid: string, id: string, range: { from: string; to: string }) =>
    ["explore", uid, "trace", id, range] as const,
  facets: (uid: string, params: LogFacetsParams) =>
    ["explore", uid, "facets", params] as const,
  patterns: (uid: string, params: LogPatternsParams) =>
    ["explore", uid, "patterns", params] as const,
  traceStats: (uid: string, params: TraceStatsParams) =>
    ["explore", uid, "trace-stats", params] as const,
  traceCompare: (uid: string, params: TraceCompareParams) =>
    ["explore", uid, "trace-compare", params] as const,
  traceLogs: (uid: string, params: TraceLogsParams) =>
    ["explore", uid, "trace-logs", params] as const,
  targets: (search: string) => ["explore", "targets", search] as const,
}

export const threadKeys = {
  all: ["threads"] as const,
  list: (target: string) => ["threads", target] as const,
}

export const findingKeys = {
  all: ["findings"] as const,
  one: (id: string) => ["findings", id] as const,
  forDashboard: (dashboardId: string) =>
    ["findings", "dashboard", dashboardId] as const,
  inFolder: (folderId: string) => ["findings", "folder", folderId] as const,
  panelData: (
    id: string,
    panelId: string,
    params: { from?: string; to?: string; maxDataPoints?: number }
  ) => ["findings", id, "panels", panelId, params] as const,
}

export type SavedSearchListParams = {
  kind?: "logs" | "traces" | "metrics"
  folderId?: string
}

export const savedSearchKeys = {
  all: ["saved-searches"] as const,
  list: (params: SavedSearchListParams) =>
    ["saved-searches", "list", params] as const,
}

export const annotationKeys = {
  all: ["annotations"] as const,
  ranges: (dashboardId: string) =>
    ["annotations", dashboardId, "range"] as const,
  range: (dashboardId: string, from: string, to: string) =>
    ["annotations", dashboardId, "range", from, to] as const,
  activity: (dashboardId: string) =>
    ["annotations", dashboardId, "activity"] as const,
}

export type HomeSearch = { q?: string; tag: string[] }

export const searchKeys = {
  all: ["search"] as const,
  results: (q: string) => ["search", "results", q] as const,
  page: (q: string, kind: string | null) =>
    ["search", "page", q, kind] as const,
}

export const pinKeys = {
  all: ["pins"] as const,
  list: () => ["pins", "list"] as const,
}

export const homeDashboardKeys = {
  current: () => ["home-dashboard"] as const,
}

export const homeKeys = {
  firstDashboards: () => ["dashboards", "list", "home-first"] as const,
  search: (search: HomeSearch) =>
    ["dashboards", "list", "home-search", search] as const,
  working: () => [...observeKeys.mineAll(), "working"] as const,
  latestChange: (observationId: string) =>
    [...observationKeys.one(observationId), "latest-change"] as const,
}

export const connectKeys = {
  request: (id: string) => ["connect", "requests", id] as const,
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

export type ObservationListFilters = {
  state?: "open" | "resolved"
  folderId?: string
}

export type ObservationPageParams = {
  state?: "open" | "resolved"
  q?: string
  mine?: boolean
  sort: "updated" | "created" | "title"
  order: "asc" | "desc"
  page: number
  size: number
}

export const observationKeys = {
  all: ["observations"] as const,
  lists: () => ["observations", "list"] as const,
  page: (params: ObservationPageParams) =>
    ["observations", "list", "page", params] as const,
  list: (filters: ObservationListFilters, limit: number) =>
    ["observations", "list", filters, limit] as const,
  one: (id: string) => ["observations", id] as const,
  blockData: (blockId: string, version: number, params: PanelDataParams) =>
    ["observations", "blocks", blockId, version, "data", params] as const,
  snapshot: (snapshotId: string) =>
    ["observations", "snapshots", snapshotId] as const,
  changes: (id: string, updatedAt: string) =>
    ["observations", id, "changes", updatedAt] as const,
  allChanges: (id: string) => ["observations", id, "changes"] as const,
  rcas: (id: string) => ["observations", id, "rcas"] as const,
  rca: (id: string, revision: number) =>
    ["observations", id, "rcas", revision] as const,
  rcaLinks: (id: string, revision: number) =>
    ["observations", id, "rcas", revision, "links"] as const,
}

export const rcaKeys = {
  publicView: (token: string) => ["rca-public", token] as const,
  publicSnapshot: (token: string, snapshotId: string) =>
    ["rca-public", token, "snapshots", snapshotId] as const,
}

export type ProposalState = "open" | "merged" | "discarded" | "all"

export const proposalKeys = {
  all: ["proposals"] as const,
  list: (dashboardId: string, state: ProposalState) =>
    ["proposals", "list", dashboardId, state] as const,
  detail: (id: string) => ["proposals", "detail", id] as const,
  run: (dashboardId: string, runId: string) =>
    ["proposals", "run", dashboardId, runId] as const,
}

export type InboxView = "for_you" | "everything" | "proposals" | "mentions"

export type InboxFilter = "open" | "unread" | "done"

export const inboxKeys = {
  all: ["inbox"] as const,
  list: (view: InboxView, filter: InboxFilter = "open") =>
    ["inbox", "list", view, filter] as const,
}

export const agentKeys = {
  all: ["agents"] as const,
  list: () => ["agents", "list"] as const,
}

export type AuditWindowKey = { q: string; since: string; until: string }

export const auditKeys = {
  all: ["audit"] as const,
  events: (window: AuditWindowKey) => ["audit", "events", window] as const,
  facets: (window: AuditWindowKey, field: string) =>
    ["audit", "facets", window, field] as const,
  histogram: (window: AuditWindowKey) =>
    ["audit", "histogram", window] as const,
  fields: () => ["audit", "fields"] as const,
  event: (id: string) => ["audit", "event", id] as const,
}

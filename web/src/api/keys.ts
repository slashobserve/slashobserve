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
}

export const dashboardKeys = {
  all: ["dashboards"] as const,
  list: () => ["dashboards", "list"] as const,
  filtered: (filters: DashboardListFilters) =>
    ["dashboards", "list", filters] as const,
  titles: () => ["dashboards", "titles"] as const,
  tags: () => ["dashboards", "tags"] as const,
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
  posts: (id: string) => ["dashboards", id, "posts"] as const,
}

export const folderKeys = {
  all: ["folders"] as const,
  list: () => ["folders", "list"] as const,
  detail: (id: string) => ["folders", id] as const,
  permissions: (id: string) => ["folders", id, "permissions"] as const,
}

export const observeKeys = {
  agent: () => ["agent"] as const,
  request: (id: string) => ["observe", id] as const,
  mine: (source: "all" | "observe", limit: number) =>
    ["observe-mine", source, limit] as const,
  mineAll: () => ["observe-mine"] as const,
  create: () => ["observe-create"] as const,
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

export const outboundDomainKeys = {
  list: () => ["outbound-domains"] as const,
}

export const newsKeys = {
  feed: (url: string) => ["news", url] as const,
}

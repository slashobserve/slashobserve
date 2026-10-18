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

export const dashboardKeys = {
  all: ["dashboards"] as const,
  list: () => ["dashboards", "list"] as const,
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
  saves: (id: string) => ["dashboards", id, "saves"] as const,
}

export const observeKeys = {
  agent: () => ["agent"] as const,
  request: (id: string) => ["observe", id] as const,
}

export const serviceAccountKeys = {
  all: ["service-accounts"] as const,
  list: () => ["service-accounts", "list"] as const,
  keys: (accountId: string) => ["service-accounts", accountId, "keys"] as const,
}

export const instanceKeys = {
  info: () => ["instance"] as const,
}

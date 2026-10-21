import {
  isDataPanel,
  queryRef,
  type DataPanel,
  type DataPanelType,
  type Panel,
  type PanelQuery,
  type TextPanel,
  type Threshold,
  type Unit,
} from "@/spec"

export type EditablePanel = DataPanel | TextPanel

export function isEditablePanel(panel: Panel): panel is EditablePanel {
  return isDataPanel(panel) || panel.type === "text"
}

export type EditorDraft = {
  baseVersion: number
  base: EditablePanel
  panel: EditablePanel
}

export function startDraft(version: number, panel: EditablePanel): EditorDraft {
  return { baseVersion: version, base: panel, panel: structuredClone(panel) }
}

export type DraftAction =
  | { type: "setQuery"; index: number; query: string }
  | { type: "setLegend"; index: number; legend: string }
  | { type: "setQueryDatasource"; index: number; uid: string | null }
  | { type: "setPanelDatasource"; uid: string }
  | { type: "addQuery" }
  | { type: "removeQuery"; index: number }
  | { type: "setType"; panelType: DataPanelType }
  | { type: "setUnit"; unit: Unit | null }
  | { type: "setThresholds"; thresholds: Threshold[] | null }
  | { type: "setTitle"; title: string }
  | { type: "setDisplay"; key: string; value: unknown }
  | { type: "setContent"; content: string }
  | { type: "replace"; panel: EditablePanel }
  | { type: "reset" }
  | { type: "rebase"; version: number; panel: EditablePanel }

const maxQueries = 26

const displayKeys: Record<DataPanelType, readonly string[]> = {
  timeseries: ["legend", "legendValues", "tooltip"],
  stat: ["reduce", "colorMode", "graph", "textMode", "orientation"],
  gauge: ["reduce", "thresholdMarkers"],
  bargauge: ["reduce", "orientation"],
  table: ["header", "footer"],
  piechart: ["reduce", "pieType", "legend"],
  heatmap: ["calculate", "yScale"],
  histogram: ["bucketSize", "bucketCount"],
  "state-timeline": ["mergeValues", "showValues"],
  logs: ["wrap", "showTime", "order"],
}

export function displayKeysFor(type: DataPanelType): readonly string[] {
  return displayKeys[type]
}

export function nextRef(queries: readonly PanelQuery[]): string {
  const used = new Set(queries.map((query, index) => queryRef(query, index)))
  for (let i = 0; ; i++) {
    const candidate = i < 26 ? String.fromCharCode(65 + i) : `Q${i + 1}`
    if (!used.has(candidate)) return candidate
  }
}

function withDraftPanel(state: EditorDraft, panel: EditablePanel): EditorDraft {
  return { ...state, panel }
}

function editData(
  state: EditorDraft,
  edit: (panel: DataPanel) => void
): EditorDraft {
  if (!isDataPanel(state.panel)) return state
  const panel = structuredClone(state.panel)
  edit(panel)
  return withDraftPanel(state, panel)
}

function editQuery(
  state: EditorDraft,
  index: number,
  edit: (query: PanelQuery) => void
): EditorDraft {
  if (!isDataPanel(state.panel) || !state.panel.queries[index]) return state
  return editData(state, (panel) => edit(panel.queries[index]))
}

function addQuery(panel: DataPanel) {
  const position = panel.queries.length
  const ref = nextRef(panel.queries)
  panel.queries.push(
    ref === queryRef({ query: "" }, position)
      ? { query: "" }
      : { ref, query: "" }
  )
}

function removeQuery(panel: DataPanel, index: number) {
  panel.queries.forEach((query, i) => {
    if (i > index) query.ref = queryRef(query, i)
  })
  panel.queries.splice(index, 1)
}

function setDisplay(panel: DataPanel, key: string, value: unknown) {
  const display: Record<string, unknown> = { ...panel.display }
  if (value === undefined) delete display[key]
  else display[key] = structuredClone(value)
  if (Object.keys(display).length === 0) delete panel.display
  else panel.display = display
}

function setType(panel: DataPanel, type: DataPanelType) {
  panel.type = type
  const allowed = displayKeysFor(type)
  const kept = Object.entries({ ...panel.display }).filter(([key]) =>
    allowed.includes(key)
  )
  if (kept.length === 0) delete panel.display
  else panel.display = Object.fromEntries(kept)
}

export function draftReducer(
  state: EditorDraft,
  action: DraftAction
): EditorDraft {
  switch (action.type) {
    case "setQuery":
      return editQuery(state, action.index, (query) => {
        query.query = action.query
      })
    case "setLegend":
      return editQuery(state, action.index, (query) => {
        if (action.legend === "") delete query.legend
        else query.legend = action.legend
      })
    case "setQueryDatasource":
      return editQuery(state, action.index, (query) => {
        if (action.uid === null) delete query.datasource
        else query.datasource = action.uid
      })
    case "setPanelDatasource":
      return editData(state, (panel) => {
        panel.datasource = action.uid
      })
    case "addQuery":
      if (
        !isDataPanel(state.panel) ||
        state.panel.queries.length >= maxQueries
      ) {
        return state
      }
      return editData(state, addQuery)
    case "removeQuery":
      if (
        !isDataPanel(state.panel) ||
        state.panel.queries.length < 2 ||
        !state.panel.queries[action.index]
      ) {
        return state
      }
      return editData(state, (panel) => removeQuery(panel, action.index))
    case "setType":
      return editData(state, (panel) => setType(panel, action.panelType))
    case "setUnit":
      return editData(state, (panel) => {
        if (action.unit === null) delete panel.unit
        else panel.unit = action.unit
      })
    case "setThresholds":
      return editData(state, (panel) => {
        if (action.thresholds === null || action.thresholds.length === 0) {
          delete panel.thresholds
        } else {
          panel.thresholds = structuredClone(action.thresholds)
        }
      })
    case "setTitle": {
      const panel = structuredClone(state.panel)
      panel.title = action.title
      return withDraftPanel(state, panel)
    }
    case "setDisplay":
      return editData(state, (panel) =>
        setDisplay(panel, action.key, action.value)
      )
    case "setContent": {
      if (state.panel.type !== "text") return state
      const panel = structuredClone(state.panel)
      panel.content = action.content
      return withDraftPanel(state, panel)
    }
    case "replace":
      return withDraftPanel(state, structuredClone(action.panel))
    case "reset":
      return withDraftPanel(state, structuredClone(state.base))
    case "rebase":
      return startDraft(action.version, action.panel)
  }
}

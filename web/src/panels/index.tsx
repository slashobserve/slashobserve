import { TablePanel } from "./table-panel"
import type { PanelRegistry } from "./types"

export const panelRenderers: PanelRegistry = {
  timeseries: TablePanel,
  stat: TablePanel,
  table: TablePanel,
  bargauge: TablePanel,
}

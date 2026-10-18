import { BargaugePanel } from "./bargauge-panel"
import { StatPanel } from "./stat-panel"
import { TablePanel } from "./table-panel"
import type { PanelRegistry } from "./types"

export const panelRenderers: PanelRegistry = {
  timeseries: TablePanel,
  stat: StatPanel,
  table: TablePanel,
  bargauge: BargaugePanel,
}

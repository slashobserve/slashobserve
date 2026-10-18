import { BargaugePanel } from "./bargauge-panel"
import { StatPanel } from "./stat-panel"
import { TablePanel } from "./table-panel"
import { TimeseriesPanel } from "./timeseries/timeseries-panel"
import type { PanelRegistry } from "./types"

export const panelRenderers: PanelRegistry = {
  timeseries: TimeseriesPanel,
  stat: StatPanel,
  table: TablePanel,
  bargauge: BargaugePanel,
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "timeseries",
  supported: true,
  mcpApp: true,
  minWidth: 240,
  minHeight: 280,
  anchors: () => import("./anchors"),
  load: () => import("./timeseries-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "heatmap",
  supported: true,
  mcpApp: true,
  minWidth: 240,
  minHeight: 280,
  anchors: () => import("./anchors"),
  load: () => import("./heatmap-panel"),
}

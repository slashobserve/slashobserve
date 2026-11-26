import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "bargauge",
  supported: true,
  mcpApp: true,
  minWidth: 200,
  minHeight: 120,
  anchors: () => import("./anchors"),
  load: () => import("./bargauge-panel"),
}

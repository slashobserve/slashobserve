import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "gauge",
  supported: true,
  mcpApp: true,
  minWidth: 180,
  minHeight: 120,
  anchors: () => import("./anchors"),
  load: () => import("./gauge-panel"),
}

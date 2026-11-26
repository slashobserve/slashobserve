import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "piechart",
  supported: true,
  mcpApp: true,
  minWidth: 200,
  minHeight: 240,
  anchors: () => import("./anchors"),
  load: () => import("./piechart-panel"),
}

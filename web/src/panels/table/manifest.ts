import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "table",
  supported: true,
  mcpApp: true,
  minWidth: 300,
  minHeight: 300,
  anchors: () => import("./anchors"),
  load: () => import("./table-panel"),
}

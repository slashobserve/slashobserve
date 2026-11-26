import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "stat",
  supported: true,
  mcpApp: true,
  minWidth: 160,
  minHeight: 120,
  anchors: () => import("./anchors"),
  load: () => import("./stat-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "logs",
  supported: true,
  mcpApp: true,
  minWidth: 300,
  minHeight: 320,
  anchors: () => import("./anchors"),
  load: () => import("./logs-panel"),
}

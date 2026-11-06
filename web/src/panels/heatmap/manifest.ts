import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "heatmap",
  supported: true,
  minWidth: 240,
  minHeight: 280,
  load: () => import("./heatmap-panel"),
}

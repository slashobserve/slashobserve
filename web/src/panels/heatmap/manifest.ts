import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "heatmap",
  supported: false,
  load: () => import("./heatmap-panel"),
}

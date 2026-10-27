import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "heatmap",
  supported: true,
  load: () => import("./heatmap-panel"),
}

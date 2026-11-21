import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "histogram",
  supported: true,
  minWidth: 240,
  minHeight: 240,
  anchors: () => import("./anchors"),
  load: () => import("./histogram-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "barchart",
  supported: true,
  minWidth: 240,
  minHeight: 260,
  anchors: () => import("./anchors"),
  load: () => import("./barchart-panel"),
}

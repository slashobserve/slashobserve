import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "canvas",
  supported: true,
  anchors: () => import("./anchors"),
  load: () => import("./canvas-panel"),
}

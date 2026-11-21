import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "traces",
  supported: true,
  minHeight: 320,
  anchors: () => import("./anchors"),
  load: () => import("./traces-panel"),
}

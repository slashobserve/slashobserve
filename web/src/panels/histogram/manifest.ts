import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "histogram",
  supported: false,
  load: () => import("./histogram-panel"),
}

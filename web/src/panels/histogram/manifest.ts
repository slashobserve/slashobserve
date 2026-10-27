import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "histogram",
  supported: true,
  load: () => import("./histogram-panel"),
}

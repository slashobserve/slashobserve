import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "barchart",
  supported: true,
  load: () => import("./barchart-panel"),
}

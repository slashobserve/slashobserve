import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "barchart",
  supported: true,
  minWidth: 240,
  load: () => import("./barchart-panel"),
}

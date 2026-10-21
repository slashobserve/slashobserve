import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "piechart",
  supported: false,
  load: () => import("./piechart-panel"),
}

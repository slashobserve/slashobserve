import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "piechart",
  supported: true,
  load: () => import("./piechart-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "piechart",
  supported: true,
  minWidth: 200,
  minHeight: 240,
  load: () => import("./piechart-panel"),
}

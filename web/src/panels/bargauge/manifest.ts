import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "bargauge",
  supported: true,
  minWidth: 200,
  minHeight: 120,
  load: () => import("./bargauge-panel"),
}

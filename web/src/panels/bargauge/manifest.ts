import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "bargauge",
  supported: true,
  minWidth: 200,
  load: () => import("./bargauge-panel"),
}

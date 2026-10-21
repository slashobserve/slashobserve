import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "bargauge",
  supported: true,
  load: () => import("./bargauge-panel"),
}

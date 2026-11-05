import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "gauge",
  supported: true,
  minWidth: 180,
  load: () => import("./gauge-panel"),
}

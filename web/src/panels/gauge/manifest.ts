import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "gauge",
  supported: false,
  load: () => import("./gauge-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "gauge",
  supported: true,
  load: () => import("./gauge-panel"),
}

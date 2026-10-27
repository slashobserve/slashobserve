import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "trend",
  supported: true,
  load: () => import("./trend-panel"),
}

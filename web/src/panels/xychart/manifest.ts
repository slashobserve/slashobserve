import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "xychart",
  supported: true,
  load: () => import("./xychart-panel"),
}

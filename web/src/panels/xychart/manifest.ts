import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "xychart",
  supported: true,
  minWidth: 240,
  load: () => import("./xychart-panel"),
}

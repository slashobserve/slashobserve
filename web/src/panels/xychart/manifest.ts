import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "xychart",
  supported: true,
  minWidth: 240,
  minHeight: 260,
  load: () => import("./xychart-panel"),
}

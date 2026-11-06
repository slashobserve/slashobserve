import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "trend",
  supported: true,
  minWidth: 240,
  minHeight: 240,
  load: () => import("./trend-panel"),
}

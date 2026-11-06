import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "status-history",
  supported: true,
  minWidth: 240,
  minHeight: 240,
  load: () => import("./status-history-panel"),
}

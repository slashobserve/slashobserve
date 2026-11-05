import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "stat",
  supported: true,
  minWidth: 160,
  load: () => import("./stat-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "stat",
  supported: true,
  load: () => import("./stat-panel"),
}

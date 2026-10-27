import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "dashlist",
  supported: true,
  load: () => import("./dashlist-panel"),
}

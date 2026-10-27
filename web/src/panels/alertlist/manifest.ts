import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "alertlist",
  supported: true,
  load: () => import("./alertlist-panel"),
}

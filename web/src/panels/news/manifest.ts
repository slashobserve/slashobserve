import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "news",
  supported: true,
  load: () => import("./news-panel"),
}

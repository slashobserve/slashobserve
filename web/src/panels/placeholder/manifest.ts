import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "placeholder",
  supported: true,
  load: () => import("./placeholder-panel"),
}

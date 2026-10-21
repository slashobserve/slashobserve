import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "placeholder",
  supported: false,
  load: () => import("./placeholder-panel"),
}

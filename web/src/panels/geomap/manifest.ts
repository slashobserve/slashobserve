import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "geomap",
  supported: true,
  load: () => import("./geomap-panel"),
}

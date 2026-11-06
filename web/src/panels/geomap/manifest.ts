import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "geomap",
  supported: true,
  minHeight: 300,
  load: () => import("./geomap-panel"),
}

import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "table",
  supported: true,
  load: () => import("./table-panel"),
}

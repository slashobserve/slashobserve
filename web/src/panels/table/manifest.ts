import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "table",
  supported: true,
  minWidth: 300,
  load: () => import("./table-panel"),
}

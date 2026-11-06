import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "timeseries",
  supported: true,
  minWidth: 240,
  minHeight: 280,
  load: () => import("./timeseries-panel"),
}

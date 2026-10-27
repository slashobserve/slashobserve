import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "candlestick",
  supported: true,
  load: () => import("./candlestick-panel"),
}

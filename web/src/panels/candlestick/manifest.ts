import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "candlestick",
  supported: true,
  minWidth: 240,
  load: () => import("./candlestick-panel"),
}

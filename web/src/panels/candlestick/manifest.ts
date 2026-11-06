import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "candlestick",
  supported: true,
  minWidth: 240,
  minHeight: 280,
  load: () => import("./candlestick-panel"),
}

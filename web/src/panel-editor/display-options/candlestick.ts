import type { DisplayOption } from "./types"
import { fieldOption, legend, tooltip, xAxis } from "./shared"

const colorOptions = [
  { value: "ok", label: "OK" },
  { value: "info", label: "Info" },
  { value: "warning", label: "Warning" },
  { value: "critical", label: "Critical" },
]

export const candlestick: readonly DisplayOption[] = [
  {
    kind: "choice",
    key: "mode",
    label: "Mode",
    options: [
      { value: "candles+volume", label: "Candles and volume" },
      { value: "candles", label: "Candles" },
      { value: "volume", label: "Volume" },
    ],
  },
  {
    kind: "choice",
    key: "candleStyle",
    label: "Candle style",
    options: [
      { value: "candles", label: "Candles" },
      { value: "ohlcbars", label: "OHLC bars" },
    ],
  },
  {
    kind: "choice",
    key: "colorStrategy",
    label: "Color strategy",
    options: [
      { value: "open-close", label: "Open to close" },
      { value: "close-close", label: "Close to close" },
    ],
  },
  { kind: "choice", key: "upColor", label: "Up color", options: colorOptions },
  {
    kind: "choice",
    key: "downColor",
    label: "Down color",
    options: colorOptions,
  },
  fieldOption("open", "Open field", "Field named open"),
  fieldOption("high", "High field", "Field named high"),
  fieldOption("low", "Low field", "Field named low"),
  fieldOption("close", "Close field", "Field named close"),
  fieldOption("volume", "Volume field", "Field named volume"),
  legend,
  tooltip,
  xAxis,
]

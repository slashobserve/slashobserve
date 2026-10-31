export type CandlestickLegend = "none" | "upDown" | "series"

export function candlestickLegend(
  display: Record<string, unknown>
): CandlestickLegend {
  if (display.legend === "hidden") return "none"
  const values = Array.isArray(display.legendValues) ? display.legendValues : []
  return display.legendMode === "table" || values.length > 0
    ? "series"
    : "upDown"
}

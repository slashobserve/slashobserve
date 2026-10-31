import uPlot from "uplot"

export function zonedPlotDate(
  zone: string | undefined
): Pick<uPlot.Options, "tzDate"> {
  if (zone === undefined) return {}
  return { tzDate: (ts) => uPlot.tzDate(new Date(ts * 1000), zone) }
}

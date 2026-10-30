import type { FieldProps } from "@/spec"

export type ColorScheme = NonNullable<FieldProps["colorScheme"]>

type Ramp =
  { kind: "hue"; hue: string } | { kind: "path"; stops: readonly string[] }

const background = "--background"
const hueFloorPercent = 15

const ok = "--threshold-ok"
const warning = "--threshold-warning"
const critical = "--threshold-critical"
const blue = "--series-1"
const orange = "--series-2"
const purple = "--series-7"

const ramps: Record<ColorScheme, Ramp> = {
  greenYellowRed: { kind: "path", stops: [ok, warning, critical] },
  redYellowGreen: { kind: "path", stops: [critical, warning, ok] },
  blueYellowRed: { kind: "path", stops: [blue, warning, critical] },
  yellowRed: { kind: "path", stops: [warning, critical] },
  bluePurple: { kind: "path", stops: [blue, purple] },
  yellowBlue: { kind: "path", stops: [warning, blue] },
  blues: { kind: "hue", hue: blue },
  reds: { kind: "hue", hue: critical },
  greens: { kind: "hue", hue: ok },
  purples: { kind: "hue", hue: purple },
  oranges: { kind: "hue", hue: orange },
  viridis: { kind: "path", stops: [purple, "--series-3", warning] },
  magma: { kind: "path", stops: [purple, critical, warning] },
  plasma: { kind: "path", stops: [purple, "--series-5", warning] },
  inferno: { kind: "path", stops: [purple, critical, warning] },
  cividis: { kind: "path", stops: [blue, "--muted-foreground", warning] },
}

export const schemes = Object.keys(ramps) as ColorScheme[]

function mix(from: string, to: string, percentOfTo: number): string {
  if (percentOfTo <= 0) return `var(${from})`
  if (percentOfTo >= 100) return `var(${to})`
  return `color-mix(in oklab, var(${to}) ${percentOfTo}%, var(${from}))`
}

export function schemeColor(scheme: ColorScheme, t: number): string {
  const position = Number.isFinite(t) ? Math.min(1, Math.max(0, t)) : 0
  const ramp = ramps[scheme]
  if (ramp.kind === "hue") {
    const percent = hueFloorPercent + (100 - hueFloorPercent) * position
    return mix(background, ramp.hue, Math.round(percent))
  }
  const segments = ramp.stops.length - 1
  const scaled = position * segments
  const index = Math.min(Math.floor(scaled), segments - 1)
  return mix(
    ramp.stops[index],
    ramp.stops[index + 1],
    Math.round((scaled - index) * 100)
  )
}

export function rangePosition(value: number, min: number, max: number): number {
  if (max <= min) return 1
  return Math.min(1, Math.max(0, (value - min) / (max - min)))
}

export type LogBase = 2 | 10

export type ScaleKind = "linear" | "log" | "symlog"

export type ScaleDistribution = {
  distr?: 3 | 4
  log?: LogBase
  asinh?: number
}

export function softRange(
  [lo, hi]: [number, number],
  softMin?: number,
  softMax?: number
): [number, number] {
  return [
    softMin === undefined ? lo : Math.min(lo, softMin),
    softMax === undefined ? hi : Math.max(hi, softMax),
  ]
}

export function centeredRange([lo, hi]: [number, number]): [number, number] {
  const reach = Math.max(Math.abs(lo), Math.abs(hi))
  return [-reach, reach]
}

function power(base: LogBase, exponent: number): number {
  return exponent < 0 ? 1 / base ** -exponent : base ** exponent
}

export function logRange(
  min: number,
  max: number,
  base: LogBase
): [number, number] {
  if (!Number.isFinite(max) || max <= 0) return [1, base]
  const exponentOf = base === 2 ? Math.log2 : Math.log10
  const lowest = Number.isFinite(min) && min > 0 ? min : max
  const lowExponent = Math.floor(exponentOf(lowest))
  const highExponent = Math.max(Math.ceil(exponentOf(max)), lowExponent + 1)
  return [power(base, lowExponent), power(base, highExponent)]
}

export function scaleDistribution(
  scale: ScaleKind | undefined,
  logBase: LogBase | undefined,
  linearThreshold: number | undefined
): ScaleDistribution {
  if (scale === "log") return { distr: 3, log: logBase === 2 ? 2 : 10 }
  if (scale === "symlog") return { distr: 4, asinh: linearThreshold ?? 1 }
  return {}
}

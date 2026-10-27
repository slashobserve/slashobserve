import { formatDateTime, formatRelative } from "./time"

type Rule =
  | { kind: "plain" }
  | { kind: "append"; suffix: string; multiplier?: number }
  | { kind: "prefix"; prefix: string }
  | { kind: "scale"; base: 1000 | 1024; suffixes: string[]; start: number }
  | { kind: "short"; suffix: string; steps?: string[] }
  | { kind: "duration"; unitNs: number }
  | { kind: "custom"; format: (value: number, decimals?: number) => string }

const shortSteps = ["", "K", "Mil", "Bil", "Tri"]
const siSteps = ["", "k", "M", "G", "T"]

const iecBytes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"]
const iecBits = ["b", "Kib", "Mib", "Gib", "Tib", "Pib"]
const siBytes = ["B", "kB", "MB", "GB", "TB", "PB"]
const siBits = ["b", "kb", "Mb", "Gb", "Tb", "Pb"]
const iecByteRate = iecBytes.map((step) => `${step}/s`)
const iecBitRate = iecBits.map((step) => `${step}/s`)
const siByteRate = siBytes.map((step) => `${step}/s`)
const siBitRate = siBits.map((step) => `${step}/s`)

const durationSteps = [
  { ns: 1, label: "ns" },
  { ns: 1e3, label: "µs" },
  { ns: 1e6, label: "ms" },
  { ns: 1e9, label: "s" },
  { ns: 6e10, label: "min" },
  { ns: 3.6e12, label: "hour" },
  { ns: 8.64e13, label: "day" },
  { ns: 6.048e14, label: "week" },
  { ns: 3.15576e16, label: "year" },
]

const decimalFormats = new Map<string, Intl.NumberFormat>()

function autoDecimals(value: number): number {
  const abs = Math.abs(value)
  if (abs === 0 || abs >= 0.01) return 2
  return Math.min(Math.ceil(-Math.log10(abs)) + 1, 8)
}

function formatNumber(value: number, decimals?: number): string {
  const max = decimals ?? autoDecimals(value)
  const min = decimals ?? 0
  const key = `${min}:${max}`
  let format = decimalFormats.get(key)
  if (!format) {
    format = new Intl.NumberFormat("en", {
      minimumFractionDigits: min,
      maximumFractionDigits: max,
      signDisplay: "negative",
    })
    decimalFormats.set(key, format)
  }
  return format.format(value)
}

type Placed = { amount: number; prefix?: string; suffix?: string }

function placeScaled(
  value: number,
  base: number,
  suffixes: string[],
  start: number
): Placed {
  let step = start
  let current = value
  while (Math.abs(current) >= base && step < suffixes.length - 1) {
    current /= base
    step++
  }
  const suffix = suffixes[step]
  return { amount: current, suffix: suffix ? ` ${suffix}` : "" }
}

function placeShort(value: number, steps: string[], tail: string): Placed {
  let step = 0
  let current = value
  while (Math.abs(current) >= 1000 && step < steps.length - 1) {
    current /= 1000
    step++
  }
  const magnitude = steps[step] ? ` ${steps[step]}` : ""
  return { amount: current, suffix: `${magnitude}${tail}` }
}

function pad(value: number, width = 2): string {
  return String(value).padStart(width, "0")
}

function placeDuration(value: number, unitNs: number): Placed {
  const ns = value * unitNs
  if (ns === 0) {
    const native = durationSteps.findLast((step) => step.ns <= unitNs)
    return { amount: 0, suffix: ` ${native?.label ?? "ns"}` }
  }
  const abs = Math.abs(ns)
  const step = durationSteps.findLast((s) => abs >= s.ns) ?? durationSteps[0]
  return { amount: ns / step.ns, suffix: ` ${step.label}` }
}

const humanDurationParts = [
  { label: "y", ms: 365 * 86_400_000 },
  { label: "d", ms: 86_400_000 },
  { label: "h", ms: 3_600_000 },
  { label: "m", ms: 60_000 },
  { label: "s", ms: 1000 },
  { label: "ms", ms: 1 },
]

function formatHumanDuration(ms: number): string {
  let rest = Math.round(Math.abs(ms))
  const parts: string[] = []
  for (const part of humanDurationParts) {
    if (parts.length === 3) break
    const count = Math.floor(rest / part.ms)
    rest -= count * part.ms
    if (count > 0) parts.push(`${count}${part.label}`)
  }
  const text = parts.length > 0 ? parts.join(" ") : "0s"
  return ms < 0 ? `-${text}` : text
}

function formatClock(totalSeconds: number): string {
  const abs = Math.floor(Math.abs(totalSeconds))
  const hours = Math.floor(abs / 3600)
  const minutes = Math.floor((abs % 3600) / 60)
  const seconds = abs % 60
  const text = `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`
  return totalSeconds < 0 ? `-${text}` : text
}

function isToday(date: Date): boolean {
  return date.toDateString() === new Date().toDateString()
}

function dateRule(
  render: (date: Date, timeOnly: boolean) => string,
  noDateIfToday = false
): Rule {
  return {
    kind: "custom",
    format: (value) => {
      const date = new Date(value)
      if (Number.isNaN(date.getTime())) return "-"
      return render(date, noDateIfToday && isToday(date))
    },
  }
}

const isoDate = (date: Date, timeOnly: boolean) =>
  timeOnly ? formatDateTime(date).slice(11) : formatDateTime(date)
const localDate = (date: Date, timeOnly: boolean) =>
  timeOnly ? date.toLocaleTimeString() : date.toLocaleString()
const usDate = (date: Date, timeOnly: boolean) =>
  timeOnly ? date.toLocaleTimeString("en-US") : date.toLocaleString("en-US")

function boolRule(no: string, yes: string): Rule {
  return { kind: "custom", format: (value) => (value === 0 ? no : yes) }
}

function hexRule(prefix: string): Rule {
  return {
    kind: "custom",
    format: (value) => {
      const rounded = Math.round(value)
      const digits = Math.abs(rounded).toString(16).toUpperCase()
      return `${rounded < 0 ? "-" : ""}${prefix}${digits}`
    },
  }
}

const currencyFormats = new Map<string, Intl.NumberFormat>()

function currencyRule(currency: string): Rule {
  return {
    kind: "custom",
    format: (value, decimals) => {
      const compact = Math.abs(value) >= 10_000
      const key = `${currency}:${compact}:${decimals ?? ""}`
      let format = currencyFormats.get(key)
      if (!format) {
        const options: Intl.NumberFormatOptions = {
          style: "currency",
          currency,
          signDisplay: "negative",
        }
        if (compact) {
          options.notation = "compact"
          options.minimumFractionDigits = 0
          options.maximumFractionDigits = decimals ?? 2
        } else if (decimals !== undefined) {
          options.minimumFractionDigits = decimals
          options.maximumFractionDigits = decimals
        }
        format = new Intl.NumberFormat("en", options)
        currencyFormats.set(key, format)
      }
      return format.format(value)
    },
  }
}

const append = (suffix: string): Rule => ({ kind: "append", suffix })
const spaced = (symbol: string): Rule => append(` ${symbol}`)
const throughput = (suffix: string): Rule => ({ kind: "short", suffix })
const scale = (base: 1000 | 1024, suffixes: string[], start = 0): Rule => ({
  kind: "scale",
  base,
  suffixes,
  start,
})
const duration = (unitNs: number): Rule => ({ kind: "duration", unitNs })

const rules = {
  none: { kind: "plain" },
  short: { kind: "short", suffix: "" },
  sishort: { kind: "short", suffix: "", steps: siSteps },
  percent: append("%"),
  percentunit: { kind: "append", suffix: "%", multiplier: 100 },
  humidity: spaced("%H"),
  dB: spaced("dB"),
  hex0x: hexRule("0x"),
  hex: hexRule(""),
  sci: {
    kind: "custom",
    format: (value, decimals) => value.toExponential(decimals ?? 2),
  },
  locale: { kind: "custom", format: (value) => value.toLocaleString() },
  pixel: spaced("px"),
  string: { kind: "custom", format: (value) => String(value) },
  bool: boolRule("false", "true"),
  bool_yes_no: boolRule("No", "Yes"),
  bool_on_off: boolRule("Off", "On"),
  bits: scale(1024, iecBits),
  bytes: scale(1024, iecBytes),
  kbytes: scale(1024, iecBytes, 1),
  mbytes: scale(1024, iecBytes, 2),
  gbytes: scale(1024, iecBytes, 3),
  tbytes: scale(1024, iecBytes, 4),
  pbytes: scale(1024, iecBytes, 5),
  decbits: scale(1000, siBits),
  decbytes: scale(1000, siBytes),
  deckbytes: scale(1000, siBytes, 1),
  decmbytes: scale(1000, siBytes, 2),
  decgbytes: scale(1000, siBytes, 3),
  dectbytes: scale(1000, siBytes, 4),
  decpbytes: scale(1000, siBytes, 5),
  pps: throughput(" p/s"),
  binBps: scale(1024, iecByteRate),
  Bps: scale(1000, siByteRate),
  binbps: scale(1024, iecBitRate),
  bps: scale(1000, siBitRate),
  KiBs: scale(1024, iecByteRate, 1),
  Kibits: scale(1024, iecBitRate, 1),
  KBs: scale(1000, siByteRate, 1),
  Kbits: scale(1000, siBitRate, 1),
  MiBs: scale(1024, iecByteRate, 2),
  Mibits: scale(1024, iecBitRate, 2),
  MBs: scale(1000, siByteRate, 2),
  Mbits: scale(1000, siBitRate, 2),
  GiBs: scale(1024, iecByteRate, 3),
  Gibits: scale(1024, iecBitRate, 3),
  GBs: scale(1000, siByteRate, 3),
  Gbits: scale(1000, siBitRate, 3),
  TiBs: scale(1024, iecByteRate, 4),
  Tibits: scale(1024, iecBitRate, 4),
  TBs: scale(1000, siByteRate, 4),
  Tbits: scale(1000, siBitRate, 4),
  cps: throughput(" c/s"),
  ops: throughput(" ops/s"),
  reqps: throughput(" req/s"),
  rps: throughput(" rd/s"),
  wps: throughput(" wr/s"),
  iops: throughput(" io/s"),
  eps: throughput(" evt/s"),
  mps: throughput(" msg/s"),
  recps: throughput(" rec/s"),
  rowsps: throughput(" rows/s"),
  cpm: throughput(" c/min"),
  opm: throughput(" ops/min"),
  rpm: throughput(" rd/min"),
  wpm: throughput(" wr/min"),
  hertz: scale(1000, ["Hz", "kHz", "MHz", "GHz", "THz"]),
  ns: duration(1),
  µs: duration(1e3),
  ms: duration(1e6),
  s: duration(1e9),
  m: duration(6e10),
  h: duration(3.6e12),
  d: duration(8.64e13),
  dtdurationms: {
    kind: "custom",
    format: (value) => formatHumanDuration(value),
  },
  dtdurations: {
    kind: "custom",
    format: (value) => formatHumanDuration(value * 1000),
  },
  dthms: spaced("dthms"),
  dtdhms: spaced("dtdhms"),
  timeticks: spaced("timeticks"),
  clockms: { kind: "custom", format: (value) => formatClock(value / 1000) },
  clocks: { kind: "custom", format: (value) => formatClock(value) },
  dateTimeAsIso: dateRule(isoDate),
  dateTimeAsIsoNoDateIfToday: dateRule(isoDate, true),
  dateTimeAsUS: dateRule(usDate),
  dateTimeAsUSNoDateIfToday: dateRule(usDate, true),
  dateTimeAsLocal: dateRule(localDate),
  dateTimeAsLocalNoDateIfToday: dateRule(localDate, true),
  dateTimeAsSystem: dateRule(localDate),
  dateTimeFromNow: {
    kind: "custom",
    format: (value) => {
      const date = new Date(value)
      if (Number.isNaN(date.getTime())) return "-"
      return formatRelative(date.toISOString())
    },
  },
  watt: scale(1000, ["W", "kW", "MW", "GW", "TW"]),
  kwatt: spaced("kW"),
  mwatt: spaced("mW"),
  watth: spaced("Wh"),
  kwatth: spaced("kWh"),
  joule: spaced("J"),
  amp: spaced("A"),
  mamp: spaced("mA"),
  volt: spaced("V"),
  mvolt: spaced("mV"),
  dBm: spaced("dBm"),
  celsius: spaced("°C"),
  fahrenheit: spaced("°F"),
  kelvin: spaced("K"),
  currencyUSD: currencyRule("USD"),
  currencyEUR: currencyRule("EUR"),
  currencyGBP: currencyRule("GBP"),
  currencyJPY: currencyRule("JPY"),
  currencyINR: currencyRule("INR"),
  lengthmm: spaced("mm"),
  lengthm: spaced("m"),
  lengthkm: spaced("km"),
  massg: spaced("g"),
  masskg: spaced("kg"),
} satisfies Record<string, Rule>

export type UnitId = keyof typeof rules

export type ParsedUnit =
  | { kind: "id"; id: UnitId }
  | { kind: "suffix"; text: string }
  | { kind: "prefix"; text: string }

const plainUnit: ParsedUnit = { kind: "id", id: "none" }

export function parseUnit(unit: string | undefined): ParsedUnit {
  if (!unit) return plainUnit
  if (Object.hasOwn(rules, unit)) return { kind: "id", id: unit as UnitId }
  if (unit.startsWith("suffix:")) {
    const text = unit.slice("suffix:".length).trim()
    return text ? { kind: "suffix", text } : plainUnit
  }
  if (unit.startsWith("prefix:")) {
    const text = unit.slice("prefix:".length)
    return text ? { kind: "prefix", text } : plainUnit
  }
  return plainUnit
}

function ruleFor(unit: string | undefined): Rule {
  const parsed = parseUnit(unit)
  switch (parsed.kind) {
    case "id":
      return rules[parsed.id]
    case "suffix":
      return { kind: "append", suffix: ` ${parsed.text}` }
    case "prefix":
      return { kind: "prefix", prefix: parsed.text }
  }
}

function place(value: number, rule: Exclude<Rule, { kind: "custom" }>): Placed {
  switch (rule.kind) {
    case "plain":
      return { amount: value }
    case "append":
      return {
        amount: value * (rule.multiplier ?? 1),
        suffix: rule.suffix,
      }
    case "prefix":
      return { amount: value, prefix: rule.prefix }
    case "scale":
      return placeScaled(value, rule.base, rule.suffixes, rule.start)
    case "short":
      return placeShort(value, rule.steps ?? shortSteps, rule.suffix)
    case "duration":
      return placeDuration(value, rule.unitNs)
  }
}

export type ValueParts = { prefix: string; amount: string; suffix: string }

export function formatValueParts(
  value: number | null | undefined,
  unit?: string,
  decimals?: number
): ValueParts {
  const whole = (amount: string) => ({ prefix: "", amount, suffix: "" })
  if (value === null || value === undefined || Number.isNaN(value)) {
    return whole("-")
  }
  if (value === Infinity) return whole("+Inf")
  if (value === -Infinity) return whole("-Inf")
  const rule = ruleFor(unit)
  if (rule.kind === "custom") return whole(rule.format(value, decimals))
  const placed = place(value, rule)
  return {
    prefix: placed.prefix ?? "",
    amount: formatNumber(placed.amount, decimals),
    suffix: placed.suffix ?? "",
  }
}

export function formatValue(
  value: number | null | undefined,
  unit?: string,
  decimals?: number
): string {
  const { prefix, amount, suffix } = formatValueParts(value, unit, decimals)
  return `${prefix}${amount}${suffix}`
}

const significantFigures = 4

export function commonDecimals(
  values: readonly (number | null | undefined)[],
  unit?: string,
  decimals?: number
): number | undefined {
  if (decimals !== undefined) return decimals
  const rule = ruleFor(unit)
  if (rule.kind === "custom") return undefined
  let widest: number | undefined
  let largest = 0
  for (const value of values) {
    if (typeof value !== "number" || !Number.isFinite(value)) continue
    const amount = place(value, rule).amount
    const text = formatNumber(amount)
    const dot = text.indexOf(".")
    const digits = dot < 0 ? 0 : text.length - dot - 1
    widest = Math.max(widest ?? 0, digits)
    largest = Math.max(largest, Math.abs(amount))
  }
  if (widest === undefined || largest === 0) return widest
  return Math.min(
    widest,
    Math.max(0, significantFigures - 1 - Math.floor(Math.log10(largest)))
  )
}

export type DecimalsItem = {
  unit?: string
  decimals?: number
  value: number | null
}

export function decimalsByUnit(
  items: readonly DecimalsItem[]
): Map<string | undefined, number | undefined> {
  const groups = new Map<string | undefined, (number | null)[]>()
  for (const { unit, decimals, value } of items) {
    const numbers = groups.get(unit) ?? []
    groups.set(unit, numbers)
    if (decimals === undefined) numbers.push(value)
  }
  return new Map(
    [...groups].map(([unit, numbers]) => [unit, commonDecimals(numbers, unit)])
  )
}

const maxOwnDecimals = 8
const noiseDigits = 12

export function ownDecimals(value: number, unit?: string): number | undefined {
  const rule = ruleFor(unit)
  if (rule.kind === "custom" || !Number.isFinite(value)) return undefined
  const amount = place(value, rule).amount
  if (!Number.isFinite(amount)) return undefined
  const clean = Number(amount.toPrecision(noiseDigits))
  for (let digits = 0; digits < maxOwnDecimals; digits++) {
    if (Number(clean.toFixed(digits)) === clean) return digits
  }
  return maxOwnDecimals
}

const maxAxisDecimals = 8

export function axisDecimals(
  splits: readonly (number | null | undefined)[],
  unit?: string,
  decimals?: number
): number | undefined {
  const base = commonDecimals(splits, unit, decimals)
  if (decimals !== undefined || base === undefined) return base
  const finite = splits.filter(
    (split): split is number =>
      typeof split === "number" && Number.isFinite(split)
  )
  for (let digits = base; digits < maxAxisDecimals; digits++) {
    const labels = finite.map((split) => formatValue(split, unit, digits))
    if (
      labels.every((label, index) => index === 0 || label !== labels[index - 1])
    ) {
      return digits
    }
  }
  return maxAxisDecimals
}

export const formatAxisValue = formatValue

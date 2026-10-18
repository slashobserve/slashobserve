import type { Unit } from "@/spec"
import { formatDateTime, formatRelative } from "./time"

type Rule =
  | { kind: "plain" }
  | { kind: "append"; suffix: string; multiplier?: number }
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

function scaled(
  value: number,
  base: number,
  suffixes: string[],
  start: number,
  decimals?: number
): string {
  let step = start
  let current = value
  while (Math.abs(current) >= base && step < suffixes.length - 1) {
    current /= base
    step++
  }
  const suffix = suffixes[step]
  return suffix
    ? `${formatNumber(current, decimals)} ${suffix}`
    : formatNumber(current, decimals)
}

function scaledSuffix(
  value: number,
  steps: string[],
  tail: string,
  decimals?: number
): string {
  let step = 0
  let current = value
  while (Math.abs(current) >= 1000 && step < steps.length - 1) {
    current /= 1000
    step++
  }
  const prefix = steps[step] ? ` ${steps[step]}` : ""
  return `${formatNumber(current, decimals)}${prefix}${tail}`
}

function pad(value: number, width = 2): string {
  return String(value).padStart(width, "0")
}

function formatDuration(value: number, unitNs: number, decimals?: number) {
  const ns = value * unitNs
  if (ns === 0) {
    const native = durationSteps.findLast((step) => step.ns <= unitNs)
    return `0 ${native?.label ?? "ns"}`
  }
  const abs = Math.abs(ns)
  const step = durationSteps.findLast((s) => abs >= s.ns) ?? durationSteps[0]
  return `${formatNumber(ns / step.ns, decimals)} ${step.label}`
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

const rules: Record<Unit, Rule> = {
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
}

export function formatValue(
  value: number | null | undefined,
  unit?: Unit,
  decimals?: number
): string {
  if (value === null || value === undefined || Number.isNaN(value)) return "-"
  if (value === Infinity) return "+Inf"
  if (value === -Infinity) return "-Inf"
  const rule = rules[unit ?? "none"]
  switch (rule.kind) {
    case "plain":
      return formatNumber(value, decimals)
    case "append":
      return `${formatNumber(value * (rule.multiplier ?? 1), decimals)}${rule.suffix}`
    case "scale":
      return scaled(value, rule.base, rule.suffixes, rule.start, decimals)
    case "short":
      return scaledSuffix(
        value,
        rule.steps ?? shortSteps,
        rule.suffix,
        decimals
      )
    case "duration":
      return formatDuration(value, rule.unitNs, decimals)
    case "custom":
      return rule.format(value, decimals)
  }
}

export const formatAxisValue = formatValue

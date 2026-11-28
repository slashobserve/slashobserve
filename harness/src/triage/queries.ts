import { compareBytes, goQuote } from "../brief/gofmt"
import { minute, second } from "./types"

export const basisSpanMetrics = "span metrics"
export const basisHTTPMetrics = "HTTP server metrics"
export const basisTraceSpans = "trace spans"
export const basisLogLevels = "log levels"

export const spanKinds = `span_kind=~"SPAN_KIND_SERVER|SPAN_KIND_CONSUMER|"`
const spanError = `status_code="STATUS_CODE_ERROR"`
export const levelLabel = "detected_level"
export const defaultLabel = "service_name"
export const traceServiceKey = "resource.service.name"
const deployPattern = "(?i)(deploy|rollout|rolled out|restart|version)"
const flagEventName = "feature_flag.evaluation"
export const flagKeyAttr = "event.feature_flag.key"
const minRateWindow = 2 * minute

export const labelName = /^[A-Za-z_][A-Za-z0-9_]*$/
export const errorLevels = [
  "error",
  "err",
  "fatal",
  "critical",
  "crit",
  "panic",
  "emerg",
  "alert",
]
export const flagVariants = [
  "event.feature_flag.result.variant",
  "event.feature_flag.variant",
]
export const logFlagKeys = [
  "feature_flag_key",
  "feature_flag_result_variant",
  "feature_flag_variant",
]

export type MetricPlan = {
  label: string
  requests: string
  errors: string[]
  bucket: string
  scale: number
  basis: string
}

function first(
  names: readonly string[],
  keep: (n: string) => boolean
): string | undefined {
  return names.find(keep)
}

function spanCallsName(calls: readonly string[]): string | undefined {
  return (
    first(calls, (n) => n.endsWith("calls_total") && n.includes("span")) ??
    first(calls, (n) => n.endsWith("calls_total"))
  )
}

function scaleOf(bucket: string): number {
  if (bucket.includes("milliseconds") || bucket === "") {
    return 1
  }
  if (bucket.includes("seconds")) {
    return 1000
  }
  return 1
}

export function pickMetrics(
  callsIn: readonly string[],
  durationsIn: readonly string[],
  label: string
): MetricPlan | undefined {
  if (!labelName.test(label)) {
    return undefined
  }
  const calls = [...callsIn].sort(compareBytes)
  const durations = [...durationsIn].sort(compareBytes)
  const span = spanCallsName(calls)
  if (span !== undefined) {
    const prefix = span.endsWith("calls_total")
      ? span.slice(0, -"calls_total".length)
      : span
    const bucket =
      first(
        durations,
        (n) =>
          n.startsWith(prefix) &&
          n.includes("duration") &&
          n.endsWith("_bucket")
      ) ?? ""
    const selector = span + "{" + spanKinds
    return {
      label,
      requests: selector + "}",
      errors: [selector + "," + spanError + "}"],
      bucket,
      scale: scaleOf(bucket),
      basis: basisSpanMetrics,
    }
  }
  const bucket = first(
    durations,
    (n) =>
      n.startsWith("http_server") &&
      n.includes("duration") &&
      n.endsWith("_bucket")
  )
  if (bucket === undefined) {
    return undefined
  }
  const count = bucket.slice(0, -"_bucket".length) + "_count"
  return {
    label,
    requests: count,
    bucket,
    scale: scaleOf(bucket),
    basis: basisHTTPMetrics,
    errors: [
      count + `{http_response_status_code=~"5.."}`,
      count + `{http_status_code=~"5.."}`,
    ],
  }
}

export function planNames(p: MetricPlan): string {
  const names = [p.requests.split("{")[0] as string]
  if (p.bucket !== "") {
    names.push(p.bucket)
  }
  return names.join(" and ")
}

export function promDuration(ms: number): string {
  return String(Math.trunc(ms / second)) + "s"
}

export function rateWindow(step: number): number {
  return Math.max(2 * step, minRateWindow)
}

export function requestQuery(p: MetricPlan, rate: number): string {
  return `sum by (${p.label}) (rate(${p.requests}[${promDuration(rate)}]))`
}

export function errorQuery(p: MetricPlan, rate: number): string {
  return p.errors
    .map((sel) => `sum by (${p.label}) (rate(${sel}[${promDuration(rate)}]))`)
    .join(" or ")
}

export function latencyQuery(p: MetricPlan, rate: number): string {
  let sel = p.bucket
  if (p.basis === basisSpanMetrics) {
    sel += "{" + spanKinds + "}"
  }
  return `histogram_quantile(0.95, sum by (${p.label}, le) (rate(${sel}[${promDuration(rate)}])))`
}

function logStreams(label: string): string {
  return `{${label}=~".+"}`
}

export function logLevelQuery(label: string, step: number): string {
  return `sum by (${label}, ${levelLabel}) (count_over_time(${logStreams(label)}[${promDuration(step)}]))`
}

export function deployLineQuery(label: string, step: number): string {
  return `sum by (${label}) (count_over_time(${logStreams(label)} |~ ${goQuote(deployPattern)} [${promDuration(step)}]))`
}

export function logFlagQuery(label: string, step: number): string {
  return `sum by (${logFlagKeys.join(", ")}) (count_over_time(${logStreams(label)} | ${logFlagKeys[0]} != "" [${promDuration(step)}]))`
}

export function lokiSelector(label: string, service: string): string {
  return `{${label}=${goQuote(service)}}`
}

export function traceAll(dsType: string): string {
  return dsType === "tempo" ? "{}" : ""
}

export function traceService(dsType: string, service: string): string {
  if (dsType === "tempo") {
    return `{ ${traceServiceKey} = ${goQuote(service)} }`
  }
  return (
    "ServiceName = '" +
    service.replaceAll("\\", "\\\\").replaceAll("'", "\\'") +
    "'"
  )
}

export function flagQuery(...conds: string[]): string {
  return (
    "{ " +
    ["event:name = " + goQuote(flagEventName), ...conds].join(" && ") +
    " }"
  )
}

export function attrEquals(attr: string, value: string): string {
  return attr + " = " + goQuote(value)
}

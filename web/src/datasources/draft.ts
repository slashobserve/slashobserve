import type { components } from "@/api/schema.gen"

export type DatasourceDraft = components["schemas"]["DatasourceDraft"]
export type DatasourceConfig = components["schemas"]["DatasourceConfig"]
export type DatasourceType = NonNullable<DatasourceDraft["type"]>
export type Audience = NonNullable<DatasourceDraft["audience"]>

export const audienceChoices: { value: Audience; label: string }[] = [
  { value: "everyone", label: "Everyone" },
  { value: "admins", label: "Only Admins" },
]

export function isAudience(value: unknown): value is Audience {
  return audienceChoices.some((choice) => choice.value === value)
}

export const datasourceTypes: { value: DatasourceType; label: string }[] = [
  { value: "prometheus", label: "Prometheus" },
  { value: "loki", label: "Loki" },
  { value: "tempo", label: "Tempo" },
  { value: "jaeger", label: "Jaeger" },
  { value: "clickhouse", label: "ClickHouse, read only" },
  { value: "elasticsearch", label: "Elasticsearch" },
  { value: "opensearch", label: "OpenSearch" },
]

export function isDatasourceType(value: string): value is DatasourceType {
  return datasourceTypes.some((type) => type.value === value)
}

export type TypeFields = {
  scrapeInterval: boolean
  tenantId: boolean
  database: boolean
  index: boolean
  bearerToken: boolean
  apiKey: boolean
  headers: boolean
}

export function fieldsOf(type: DatasourceType | ""): TypeFields {
  const http = type !== "clickhouse" && type !== ""
  const search = type === "elasticsearch" || type === "opensearch"
  return {
    scrapeInterval: type === "prometheus",
    tenantId: type === "loki",
    database: type === "clickhouse",
    index: search,
    bearerToken: http,
    apiKey: search,
    headers: http,
  }
}

export function addressHint(type: DatasourceType | ""): string {
  switch (type) {
    case "clickhouse":
      return "host:port of the native protocol, such as clickhouse:9000"
    case "prometheus":
      return "https://prometheus.example.com"
    case "loki":
      return "https://loki.example.com"
    case "tempo":
      return "https://tempo.example.com"
    case "jaeger":
      return "https://jaeger.example.com"
    case "elasticsearch":
    case "opensearch":
      return "https://search.example.com:9200"
    default:
      return "Choose a type first"
  }
}

type HeaderRow = { name: string; value: string }

export type FormValues = {
  type: DatasourceType | ""
  uid: string
  name: string
  audience: Audience
  address: string
  username: string
  database: string
  tls: boolean
  tenantId: string
  scrapeInterval: string
  index: string
  timeField: string
  queryLanguage: "" | "kql" | "lucene"
  fieldMessage: string
  fieldLevel: string
  fieldService: string
  fieldTraceId: string
  fieldSpanId: string
  timeout: string
  maxSeries: string
  maxPoints: string
  maxRows: string
  maxConcurrent: string
  grafanaUids: string
  password: string
  bearerToken: string
  apiKey: string
  headers: HeaderRow[]
  clear: string[]
}

export const emptyForm: FormValues = {
  type: "",
  uid: "",
  name: "",
  audience: "everyone",
  address: "",
  username: "",
  database: "",
  tls: false,
  tenantId: "",
  scrapeInterval: "",
  index: "",
  timeField: "",
  queryLanguage: "",
  fieldMessage: "",
  fieldLevel: "",
  fieldService: "",
  fieldTraceId: "",
  fieldSpanId: "",
  timeout: "",
  maxSeries: "",
  maxPoints: "",
  maxRows: "",
  maxConcurrent: "",
  grafanaUids: "",
  password: "",
  bearerToken: "",
  apiKey: "",
  headers: [],
  clear: [],
}

function numberText(value: number | undefined): string {
  return value === undefined || value === 0 ? "" : String(value)
}

export function formOf(config: DatasourceConfig): FormValues {
  return {
    ...emptyForm,
    type: isDatasourceType(config.type) ? config.type : "",
    uid: config.uid,
    name: config.name === config.uid ? "" : config.name,
    audience: config.audience,
    address: config.address,
    username: config.username ?? "",
    database: config.database ?? "",
    tls: config.tls ?? false,
    tenantId: config.tenant_id ?? "",
    scrapeInterval: config.scrape_interval ?? "",
    index: config.index ?? "",
    timeField: config.time_field ?? "",
    queryLanguage:
      config.query_language === "kql" || config.query_language === "lucene"
        ? config.query_language
        : "",
    fieldMessage: config.fields?.message ?? "",
    fieldLevel: config.fields?.level ?? "",
    fieldService: config.fields?.service ?? "",
    fieldTraceId: config.fields?.trace_id ?? "",
    fieldSpanId: config.fields?.span_id ?? "",
    timeout: config.limits?.timeout ?? "",
    maxSeries: numberText(config.limits?.max_series),
    maxPoints: numberText(config.limits?.max_points),
    maxRows: numberText(config.limits?.max_rows),
    maxConcurrent: numberText(config.limits?.max_concurrent_queries),
    grafanaUids: (config.grafana_uids ?? []).join(", "),
  }
}

export type SavedCredentials = {
  password: boolean
  bearerToken: boolean
  apiKey: boolean
  headers: string[]
}

export function savedCredentials(setNames: string[]): SavedCredentials {
  const prefix = "header:"
  return {
    password: setNames.includes("password"),
    bearerToken: setNames.includes("bearer_token"),
    apiKey: setNames.includes("api_key"),
    headers: setNames
      .filter((name) => name.startsWith(prefix))
      .map((name) => name.slice(prefix.length)),
  }
}

export const noCredentials: SavedCredentials = {
  password: false,
  bearerToken: false,
  apiKey: false,
  headers: [],
}

export type Problems = Record<string, string>

const uidPattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/
const maxHeaderNameCharacters = 256

function urlProblem(address: string): string | undefined {
  let protocol: string
  try {
    protocol = new URL(address).protocol
  } catch {
    return "Enter a full http or https URL, such as https://prometheus.example.com."
  }
  if (protocol !== "http:" && protocol !== "https:") {
    return "The address must start with http:// or https://."
  }
  return undefined
}

function hostPortProblem(address: string): string | undefined {
  if (
    /^[^\s/:@]+:\d{1,5}$/.test(address) ||
    /^\[[0-9a-f:.]+\]:\d{1,5}$/i.test(address)
  ) {
    return undefined
  }
  return "Enter host:port, such as clickhouse:9000."
}

function countProblem(value: string, what: string): string | undefined {
  if (value.trim() === "") return undefined
  if (!/^\d+$/.test(value.trim())) {
    return `${what} must be a whole number, or empty for the default.`
  }
  return undefined
}

export function validate(
  values: FormValues,
  mode: "create" | "edit"
): Problems {
  const problems: Problems = {}
  if (mode === "create") {
    if (values.type === "") problems.type = "Choose a type."
    const uid = values.uid.trim()
    if (uid === "") problems.uid = "Enter an id."
    else if (!uidPattern.test(uid)) {
      problems.uid =
        "Use up to 40 letters, digits, _ or -, starting with a letter or digit."
    }
  }
  const address = values.address.trim()
  if (address === "") problems.address = "Enter the address."
  else if (values.type === "clickhouse") {
    const problem = hostPortProblem(address)
    if (problem !== undefined) problems.address = problem
  } else if (values.type !== "") {
    const problem = urlProblem(address)
    if (problem !== undefined) problems.address = problem
  }
  const counts: [string, string, string][] = [
    ["limits.max_series", values.maxSeries, "Max series"],
    ["limits.max_points", values.maxPoints, "Max points"],
    ["limits.max_rows", values.maxRows, "Max rows"],
    [
      "limits.max_concurrent_queries",
      values.maxConcurrent,
      "Max concurrent queries",
    ],
  ]
  for (const [path, value, what] of counts) {
    const problem = countProblem(value, what)
    if (problem !== undefined) problems[path] = problem
  }
  const seen = new Set<string>()
  values.headers.forEach((row, index) => {
    const name = row.name.trim()
    if (name === "" && row.value === "") return
    const key = `headers.${index}`
    if (name === "") problems[key] = "Enter the header name."
    else if ([...name].length > maxHeaderNameCharacters) {
      problems[key] = "The header name is too long."
    } else if (row.value === "") problems[key] = "Enter the header value."
    else if (seen.has(name.toLowerCase())) {
      problems[key] = "This header is listed twice."
    }
    seen.add(name.toLowerCase())
  })
  return problems
}

function count(value: string): number {
  const trimmed = value.trim()
  return trimmed === "" ? 0 : Number(trimmed)
}

function grafanaUidList(text: string): string[] {
  return text
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part !== "")
}

function secretsOf(values: FormValues): DatasourceDraft["secrets"] | undefined {
  const fields = fieldsOf(values.type)
  const secrets: NonNullable<DatasourceDraft["secrets"]> = {}
  if (values.password !== "") secrets.password = values.password
  if (fields.bearerToken && values.bearerToken !== "") {
    secrets.bearer_token = values.bearerToken
  }
  if (fields.apiKey && values.apiKey !== "") secrets.api_key = values.apiKey
  if (fields.headers) {
    const headers: Record<string, string> = {}
    for (const row of values.headers) {
      const name = row.name.trim()
      if (name !== "" && row.value !== "") headers[name] = row.value
    }
    if (Object.keys(headers).length > 0) secrets.headers = headers
  }
  return Object.keys(secrets).length === 0 ? undefined : secrets
}

export function draftOf(
  values: FormValues,
  mode: "create" | "edit"
): DatasourceDraft {
  const fields = fieldsOf(values.type)
  const draft: DatasourceDraft = {
    name: values.name.trim(),
    audience: values.audience,
    address: values.address.trim(),
    username: values.username.trim(),
    grafana_uids: grafanaUidList(values.grafanaUids),
    limits: {
      timeout: values.timeout.trim(),
      max_series: count(values.maxSeries),
      max_points: count(values.maxPoints),
      max_rows: count(values.maxRows),
      max_concurrent_queries: count(values.maxConcurrent),
    },
  }
  if (mode === "create" && values.type !== "") {
    draft.uid = values.uid.trim()
    draft.type = values.type
  }
  if (fields.database) {
    draft.database = values.database.trim()
    draft.tls = values.tls
  }
  if (fields.tenantId) draft.tenant_id = values.tenantId.trim()
  if (fields.scrapeInterval)
    draft.scrape_interval = values.scrapeInterval.trim()
  if (fields.index) {
    draft.index = values.index.trim()
    draft.time_field = values.timeField.trim()
    draft.query_language = values.queryLanguage
    draft.fields = {
      message: values.fieldMessage.trim(),
      level: values.fieldLevel.trim(),
      service: values.fieldService.trim(),
      trace_id: values.fieldTraceId.trim(),
      span_id: values.fieldSpanId.trim(),
    }
  }
  const secrets = secretsOf(values)
  if (secrets !== undefined) draft.secrets = secrets
  if (values.clear.length > 0) draft.clear_secrets = values.clear
  return draft
}

export function toggleClear(clear: string[], name: string): string[] {
  return clear.includes(name)
    ? clear.filter((entry) => entry !== name)
    : [...clear, name]
}

function visiblePaths(
  type: DatasourceType | "",
  mode: "create" | "edit"
): string[] {
  const fields = fieldsOf(type)
  const paths = [
    "address",
    "name",
    "username",
    "secrets.password",
    "limits.timeout",
    "limits.max_series",
    "limits.max_points",
    "limits.max_rows",
    "limits.max_concurrent_queries",
    "grafana_uids",
  ]
  if (mode === "create") paths.push("uid", "type")
  if (fields.scrapeInterval) paths.push("scrape_interval")
  if (fields.tenantId) paths.push("tenant_id")
  if (fields.database) paths.push("database")
  if (fields.bearerToken) paths.push("secrets.bearer_token")
  if (fields.apiKey) paths.push("secrets.api_key")
  if (fields.headers) paths.push("secrets.headers")
  if (fields.index) {
    paths.push(
      "index",
      "time_field",
      "query_language",
      "fields.message",
      "fields.level",
      "fields.service",
      "fields.trace_id",
      "fields.span_id"
    )
  }
  return paths
}

function normalizePath(path: string): string {
  return path.replace(/\[\d+\]/g, "")
}

export function splitProblems(
  entries: { field: string; message: string }[],
  type: DatasourceType | "",
  mode: "create" | "edit"
): { inline: Problems; general: string[] } {
  const visible = new Set(visiblePaths(type, mode))
  const inline: Problems = {}
  const general: string[] = []
  for (const entry of entries) {
    const path = normalizePath(entry.field)
    if (visible.has(path)) {
      if (inline[path] === undefined) inline[path] = entry.message
    } else {
      general.push(`${entry.field}: ${entry.message}`)
    }
  }
  return { inline, general }
}

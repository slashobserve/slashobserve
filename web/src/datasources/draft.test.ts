import { describe, expect, test } from "vitest"

import {
  draftOf,
  emptyForm,
  fieldsOf,
  formOf,
  savedCredentials,
  splitProblems,
  toggleClear,
  validate,
  type FormValues,
} from "./draft"

function form(overrides: Partial<FormValues> = {}): FormValues {
  return {
    ...emptyForm,
    type: "prometheus",
    uid: "prom-prod",
    address: "https://prom.example.com",
    ...overrides,
  }
}

describe("validate", () => {
  test("accepts a complete prometheus datasource", () => {
    expect(validate(form(), "create")).toEqual({})
  })

  test("asks for a type, an id and an address when creating", () => {
    expect(validate(emptyForm, "create")).toEqual({
      type: "Choose a type.",
      uid: "Enter an id.",
      address: "Enter the address.",
    })
  })

  test("does not ask for a type or id when editing", () => {
    expect(validate(form({ uid: "" }), "edit")).toEqual({})
  })

  test("refuses an id the server would refuse", () => {
    expect(validate(form({ uid: "has space" }), "create").uid).toMatch(
      /up to 40/
    )
    expect(validate(form({ uid: "-x" }), "create").uid).toMatch(/up to 40/)
    expect(validate(form({ uid: "a".repeat(41) }), "create").uid).toMatch(
      /up to 40/
    )
  })

  test("wants an http url for url types and host:port for clickhouse", () => {
    expect(
      validate(form({ address: "prom.example.com" }), "create").address
    ).toMatch(/full http/)
    expect(
      validate(form({ address: "ftp://prom.example.com" }), "create").address
    ).toMatch(/http:\/\/ or https:\/\//)
    expect(
      validate(form({ type: "clickhouse", address: "ch:9000" }), "create")
    ).toEqual({})
    expect(
      validate(
        form({ type: "clickhouse", address: "https://ch.example.com" }),
        "create"
      ).address
    ).toMatch(/host:port/)
  })

  test("limits must be whole numbers or empty", () => {
    const problems = validate(
      form({ maxRows: "ten", maxSeries: "5" }),
      "create"
    )
    expect(problems["limits.max_rows"]).toMatch(/whole number/)
    expect(problems["limits.max_series"]).toBeUndefined()
  })

  test("a header needs a name and a value, and names are unique", () => {
    const problems = validate(
      form({
        headers: [
          { name: "X-A", value: "1" },
          { name: "x-a", value: "2" },
          { name: "", value: "3" },
          { name: "X-B", value: "" },
          { name: "", value: "" },
        ],
      }),
      "create"
    )
    expect(problems["headers.0"]).toBeUndefined()
    expect(problems["headers.1"]).toMatch(/twice/)
    expect(problems["headers.2"]).toMatch(/name/)
    expect(problems["headers.3"]).toMatch(/value/)
    expect(problems["headers.4"]).toBeUndefined()
  })
})

describe("draftOf", () => {
  test("a create names the uid and type and only the fields of the type", () => {
    const draft = draftOf(
      form({ scrapeInterval: "30s", tenantId: "ignored", database: "ignored" }),
      "create"
    )
    expect(draft.uid).toBe("prom-prod")
    expect(draft.type).toBe("prometheus")
    expect(draft.scrape_interval).toBe("30s")
    expect(draft).not.toHaveProperty("tenant_id")
    expect(draft).not.toHaveProperty("database")
    expect(draft).not.toHaveProperty("index")
  })

  test("sends the audience on a create and on an edit", () => {
    expect(draftOf(form(), "create").audience).toBe("everyone")
    expect(draftOf(form({ audience: "admins" }), "edit").audience).toBe(
      "admins"
    )
  })

  test("an edit leaves the uid and type out", () => {
    const draft = draftOf(form(), "edit")
    expect(draft).not.toHaveProperty("uid")
    expect(draft).not.toHaveProperty("type")
  })

  test("blank limits go out as zero so an edit can reset them", () => {
    expect(draftOf(form({ maxRows: " 50 " }), "edit").limits).toEqual({
      timeout: "",
      max_series: 0,
      max_points: 0,
      max_rows: 50,
      max_concurrent_queries: 0,
    })
  })

  test("sends credentials only when typed and the clear list as it is", () => {
    expect(draftOf(form(), "edit")).not.toHaveProperty("secrets")
    const draft = draftOf(
      form({
        password: "pw",
        bearerToken: "tok",
        headers: [
          { name: " X-Org ", value: "7" },
          { name: "", value: "" },
        ],
        clear: ["api_key", "header:Old"],
      }),
      "edit"
    )
    expect(draft.secrets).toEqual({
      password: "pw",
      bearer_token: "tok",
      headers: { "X-Org": "7" },
    })
    expect(draft.clear_secrets).toEqual(["api_key", "header:Old"])
  })

  test("never sends a bearer token or headers for clickhouse", () => {
    const draft = draftOf(
      form({
        type: "clickhouse",
        address: "ch:9000",
        password: "pw",
        bearerToken: "tok",
        headers: [{ name: "X-A", value: "1" }],
      }),
      "create"
    )
    expect(draft.secrets).toEqual({ password: "pw" })
  })

  test("search types carry the index, fields and query language", () => {
    const draft = draftOf(
      form({
        type: "elasticsearch",
        index: "logs-*",
        queryLanguage: "kql",
        fieldTraceId: "trace.id",
        apiKey: "key",
      }),
      "create"
    )
    expect(draft.index).toBe("logs-*")
    expect(draft.query_language).toBe("kql")
    expect(draft.fields?.trace_id).toBe("trace.id")
    expect(draft.secrets).toEqual({ api_key: "key" })
  })

  test("splits comma separated grafana ids", () => {
    expect(
      draftOf(form({ grafanaUids: " a, b ,,c " }), "edit").grafana_uids
    ).toEqual(["a", "b", "c"])
  })
})

describe("formOf and savedCredentials", () => {
  test("fills the form from a saved config without any credential", () => {
    const values = formOf({
      uid: "prom",
      type: "prometheus",
      name: "prom",
      audience: "everyone",
      source: "app",
      address: "https://prom.example.com",
      limits: { max_rows: 0, timeout: "20s" },
      grafana_uids: ["g1", "g2"],
      secrets_set: ["password"],
      created_at: "2026-11-19T10:00:00Z",
      updated_at: "2026-11-19T10:00:00Z",
    })
    expect(values.audience).toBe("everyone")
    expect(values.name).toBe("")
    expect(values.timeout).toBe("20s")
    expect(values.maxRows).toBe("")
    expect(values.grafanaUids).toBe("g1, g2")
    expect(values.password).toBe("")
  })

  test("reads which credentials are set from their names", () => {
    expect(
      savedCredentials(["password", "header:X-Org", "header:X-Team", "api_key"])
    ).toEqual({
      password: true,
      bearerToken: false,
      apiKey: true,
      headers: ["X-Org", "X-Team"],
    })
  })
})

describe("toggleClear", () => {
  test("adds and removes a name", () => {
    expect(toggleClear([], "password")).toEqual(["password"])
    expect(toggleClear(["password", "api_key"], "password")).toEqual([
      "api_key",
    ])
  })
})

describe("fieldsOf", () => {
  test("shows only what a type uses", () => {
    expect(fieldsOf("clickhouse")).toMatchObject({
      database: true,
      bearerToken: false,
      headers: false,
    })
    expect(fieldsOf("loki")).toMatchObject({ tenantId: true, apiKey: false })
    expect(fieldsOf("opensearch")).toMatchObject({ index: true, apiKey: true })
    expect(fieldsOf("jaeger")).toMatchObject({
      bearerToken: true,
      headers: true,
      index: false,
    })
    expect(fieldsOf("")).toMatchObject({ headers: false })
  })
})

describe("splitProblems", () => {
  test("puts a problem under its input and the rest in a list", () => {
    const split = splitProblems(
      [
        { field: "address", message: "must be an http or https URL" },
        { field: "address", message: "second" },
        { field: "grafana_uids[1]", message: "already used" },
        { field: "tenant_id", message: "not for this type" },
        { field: "definition", message: "could not be used" },
      ],
      "prometheus",
      "create"
    )
    expect(split.inline).toEqual({
      address: "must be an http or https URL",
      grafana_uids: "already used",
    })
    expect(split.general).toEqual([
      "tenant_id: not for this type",
      "definition: could not be used",
    ])
  })
})

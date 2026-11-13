import { describe, expect, it } from "vitest"

import {
  filterTarget,
  isStreamLabel,
  withFilter,
  type FilterTarget,
} from "./query-filters"

const template = `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp)
ORDER BY Timestamp DESC
LIMIT 1000`

type Case = {
  name: string
  type: "loki" | "clickhouse"
  target: FilterTarget
  query: string
  field: string
  value: string
  negate: boolean
  expected: string | null
}

const cases: Case[] = [
  {
    name: "adds a matcher inside the selector",
    type: "loki",
    target: "loki-stream",
    query: '{service_name="checkout"}',
    field: "pod",
    value: "checkout-1",
    negate: false,
    expected: '{service_name="checkout", pod="checkout-1"}',
  },
  {
    name: "adds a negative matcher inside a selector that keeps a positive one",
    type: "loki",
    target: "loki-stream",
    query: '{service_name="checkout"} |= "error"',
    field: "pod",
    value: "checkout-1",
    negate: true,
    expected: '{service_name="checkout", pod!="checkout-1"} |= "error"',
  },
  {
    name: "replaces a regex matcher on the same label instead of adding a second",
    type: "loki",
    target: "loki-stream",
    query: '{service_name=~"a|b", env="prod"} | json',
    field: "service_name",
    value: "checkout",
    negate: false,
    expected: '{service_name="checkout", env="prod"} | json',
  },
  {
    name: "fills an empty selector",
    type: "loki",
    target: "loki-stream",
    query: "{}",
    field: "service_name",
    value: "checkout",
    negate: false,
    expected: '{service_name="checkout"}',
  },
  {
    name: "refuses to leave a selector with no positive matcher",
    type: "loki",
    target: "loki-stream",
    query: '{service_name="checkout"}',
    field: "service_name",
    value: "checkout",
    negate: true,
    expected: null,
  },
  {
    name: "finds the selector inside a metric query",
    type: "loki",
    target: "loki-stream",
    query: 'sum(rate({service_name="a"}[5m]))',
    field: "pod",
    value: "p",
    negate: false,
    expected: 'sum(rate({service_name="a", pod="p"}[5m]))',
  },
  {
    name: "returns null for a query with no selector",
    type: "loki",
    target: "loki-stream",
    query: "error",
    field: "pod",
    value: "p",
    negate: false,
    expected: null,
  },
  {
    name: "escapes a double quote in a Loki value",
    type: "loki",
    target: "loki-stream",
    query: '{a="b"}',
    field: "msg",
    value: 'say "hi"',
    negate: false,
    expected: '{a="b", msg="say \\"hi\\""}',
  },
  {
    name: "returns null when the selector already holds the matcher",
    type: "loki",
    target: "loki-stream",
    query: '{service_name="checkout"}',
    field: "service_name",
    value: "checkout",
    negate: false,
    expected: null,
  },
  {
    name: "appends a label filter after the pipeline",
    type: "loki",
    target: "loki-line",
    query: '{service_name="checkout"} | json',
    field: "status",
    value: "500",
    negate: false,
    expected: '{service_name="checkout"} | json | status="500"',
  },
  {
    name: "appends a negative label filter at the very end",
    type: "loki",
    target: "loki-line",
    query: '{service_name="checkout"} | json | line_format "{{.msg}}"\n',
    field: "status",
    value: "500",
    negate: true,
    expected:
      '{service_name="checkout"} | json | line_format "{{.msg}}" | status!="500"',
  },
  {
    name: "returns null when the pipeline already holds the stage",
    type: "loki",
    target: "loki-line",
    query: '{a="b"} | json | status="500"',
    field: "status",
    value: "500",
    negate: false,
    expected: null,
  },
  {
    name: "returns null for a line filter on a metric query",
    type: "loki",
    target: "loki-line",
    query: 'sum(rate({a="b"}[5m]))',
    field: "status",
    value: "500",
    negate: false,
    expected: null,
  },
  {
    name: "returns null for a Loki name that is not a label name",
    type: "loki",
    target: "loki-line",
    query: '{a="b"}',
    field: "http.status",
    value: "500",
    negate: false,
    expected: null,
  },
  {
    name: "inserts a column condition before ORDER BY",
    type: "clickhouse",
    target: "clickhouse-column",
    query: template,
    field: "ServiceName",
    value: "checkout",
    negate: false,
    expected: template.replace(
      "$__timeFilter(Timestamp)",
      "$__timeFilter(Timestamp) AND ServiceName = 'checkout'"
    ),
  },
  {
    name: "inserts a negative condition at the end of a query with no ORDER BY",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "select * from otel_logs where Body != ''",
    field: "SeverityText",
    value: "ERROR",
    negate: true,
    expected:
      "select * from otel_logs where Body != '' AND SeverityText != 'ERROR'",
  },
  {
    name: "inserts a map attribute condition before LIMIT",
    type: "clickhouse",
    target: "clickhouse-attribute",
    query: "SELECT * FROM otel_logs WHERE 1 LIMIT 10",
    field: "http.status",
    value: "500",
    negate: false,
    expected:
      "SELECT * FROM otel_logs WHERE 1 AND LogAttributes['http.status'] = '500' LIMIT 10",
  },
  {
    name: "wraps an existing OR so the new condition binds to all of it",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "SELECT * FROM otel_logs WHERE a = 1 OR b = 2",
    field: "ServiceName",
    value: "x",
    negate: false,
    expected:
      "SELECT * FROM otel_logs WHERE (a = 1 OR b = 2) AND ServiceName = 'x'",
  },
  {
    name: "returns null for ClickHouse with no WHERE",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "SELECT * FROM otel_logs ORDER BY Timestamp",
    field: "ServiceName",
    value: "x",
    negate: false,
    expected: null,
  },
  {
    name: "returns null for ClickHouse with a subquery",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "SELECT * FROM (SELECT * FROM otel_logs WHERE 1) WHERE 1",
    field: "ServiceName",
    value: "x",
    negate: false,
    expected: null,
  },
  {
    name: "escapes a single quote and a backslash in a ClickHouse value",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "SELECT * FROM otel_logs WHERE 1",
    field: "Body",
    value: "it's a\\b",
    negate: false,
    expected: "SELECT * FROM otel_logs WHERE 1 AND Body = 'it\\'s a\\\\b'",
  },
  {
    name: "returns null for a ClickHouse attribute key with a quote",
    type: "clickhouse",
    target: "clickhouse-attribute",
    query: "SELECT * FROM otel_logs WHERE 1",
    field: "a'b",
    value: "x",
    negate: false,
    expected: null,
  },
  {
    name: "returns null when the WHERE clause already holds the condition",
    type: "clickhouse",
    target: "clickhouse-column",
    query: "SELECT * FROM otel_logs WHERE ServiceName = 'x' LIMIT 5",
    field: "ServiceName",
    value: "x",
    negate: false,
    expected: null,
  },
]

describe("withFilter", () => {
  it.each(cases)("$name", (item) => {
    expect(
      withFilter(
        item.query,
        item.type,
        item.target,
        item.field,
        item.value,
        item.negate
      )
    ).toBe(item.expected)
  })
})

describe("filterTarget", () => {
  it.each<[string, "loki" | "clickhouse", string, boolean, FilterTarget]>([
    ["a Loki stream label", "loki", "pod", true, "loki-stream"],
    ["a Loki field outside the index", "loki", "pod", false, "loki-line"],
    [
      "a ClickHouse column",
      "clickhouse",
      "TraceId",
      false,
      "clickhouse-column",
    ],
    [
      "a ClickHouse map key",
      "clickhouse",
      "http.status",
      false,
      "clickhouse-attribute",
    ],
  ])("picks the target for %s", (_label, type, name, stream, expected) => {
    expect(filterTarget(type, name, stream)).toBe(expected)
  })
})

describe("isStreamLabel", () => {
  const labels = { service_name: "a", pod: "b", trace_id: "c" }

  it.each([
    ["an indexed label", "pod", ["pod", "service_name"], true],
    ["a label outside the known index", "pod", ["service_name"], false],
    [
      "a per-line label even when listed as indexed",
      "trace_id",
      ["trace_id"],
      false,
    ],
    ["a label when the index is unknown", "pod", undefined, true],
    ["a label when the index list is empty", "pod", [], true],
    ["a name that is not a label", "status", undefined, false],
  ])("answers for %s", (_label, name, indexed, expected) => {
    expect(isStreamLabel(name, labels, indexed)).toBe(expected)
  })
})

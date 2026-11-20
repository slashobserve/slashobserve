import { useState } from "react"

import { nextMode, useInputMode, type InputMode } from "@/ask/input-mode"

import type { TraceSearch } from "./search"

const tempoModes: readonly InputMode[] = ["ask", "code"]
const tempoLabels: Record<InputMode, string> = {
  ask: "Ask",
  builder: "Builder",
  code: "Query",
}
const clickhouseModes: readonly InputMode[] = ["ask", "builder", "code"]
const clickhouseLabels: Record<InputMode, string> = {
  ask: "Ask",
  builder: "Search",
  code: "SQL",
}

function hasTraceFilters(search: TraceSearch): boolean {
  return [
    search.service,
    search.span,
    search.min,
    search.max,
    search.errors,
    search.q,
  ].some((value) => value !== undefined)
}

export function useTraceInputMode({
  type,
  search,
  agentAvailable,
}: {
  type: string
  search: TraceSearch
  agentAvailable: boolean
}) {
  const clickhouse = type === "clickhouse"
  const [mode, setMode] = useInputMode({
    agentAvailable,
    hasQuery: hasTraceFilters(search),
    hasAsk: search.ask !== undefined,
  })
  const [sql, setSql] = useState(clickhouse && search.q !== undefined)
  const [sqlFor, setSqlFor] = useState(search.q)
  if (sqlFor !== search.q) {
    setSqlFor(search.q)
    setSql(clickhouse && search.q !== undefined)
  }
  const shown: InputMode =
    mode === "ask" || !clickhouse ? mode : sql ? "code" : "builder"
  const formMode: InputMode = clickhouse ? "builder" : "code"

  function select(next: InputMode, remember = true) {
    if (clickhouse && next !== "ask") setSql(next === "code")
    setMode(next === "ask" ? "ask" : "code", remember)
  }

  return {
    shown,
    formMode,
    sqlShown: clickhouse && shown === "code",
    modes: clickhouse ? clickhouseModes : tempoModes,
    labels: clickhouse ? clickhouseLabels : tempoLabels,
    following: () => nextMode(shown, agentAvailable, clickhouse),
    select,
  }
}

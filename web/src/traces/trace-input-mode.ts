import { useState } from "react"

import {
  askModeLabel,
  nextMode,
  useInputMode,
  type InputMode,
} from "@/ask/input-mode"

import { sourceLanguage } from "@/datasources/sources"

import type { TraceSearch } from "./search"

const searchModes: readonly InputMode[] = ["code", "ask"]
const clickhouseModes: readonly InputMode[] = ["builder", "code", "ask"]
const clickhouseLabels: Record<InputMode, string> = {
  ask: askModeLabel,
  builder: "Search",
  code: "SQL",
}

function searchLabels(type: string): Record<InputMode, string> {
  return {
    ask: askModeLabel,
    builder: "Builder",
    code: sourceLanguage("traces", type),
  }
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

  const sqlShown = clickhouse && shown === "code"

  return {
    shown,
    formMode,
    sqlShown,
    language: clickhouse
      ? clickhouseLabels[sqlShown ? "code" : "builder"]
      : undefined,
    modes: clickhouse ? clickhouseModes : searchModes,
    labels: clickhouse ? clickhouseLabels : searchLabels(type),
    following: () => nextMode(shown, agentAvailable, clickhouse),
    select,
  }
}

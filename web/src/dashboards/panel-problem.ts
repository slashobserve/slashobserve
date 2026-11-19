import { ApiError, failedQueryText } from "@/api/errors"

import { VariableLoadError } from "./variables"

export type PanelProblem = {
  title: string
  message: string
  retry: boolean
  empty: boolean
}

const titles: Record<string, string> = {
  query_timeout: "Query timed out",
  datasource_unavailable: "Datasource unavailable",
  limit_exceeded: "Too much data",
  query_invalid: "Query error",
  time_range_in_future: "Time range starts in the future",
  time_range_too_long: "Time range too long",
  query_forbidden: "Query not allowed",
}

const variableName = /^variable has no values: "([^"]+)"/

function emptyVariable(error: ApiError): PanelProblem {
  const name = variableName.exec(error.message)?.[1]
  const which = name === undefined ? "a variable" : `the variable "${name}"`
  return {
    title: "No data in this range",
    message: `Nothing in this time range gives ${which} a value. Choose another range.`,
    retry: false,
    empty: true,
  }
}

export function panelProblem(error: unknown): PanelProblem {
  if (error instanceof VariableLoadError) {
    return {
      title: "Variable failed to load",
      message: error.message,
      retry: true,
      empty: false,
    }
  }
  if (error instanceof ApiError) {
    if (error.code === "variable_empty") return emptyVariable(error)
    if (error.code === "invalid_request") {
      return {
        title: "This request cannot run",
        message: failedQueryText(error),
        retry: false,
        empty: false,
      }
    }
    return {
      title: titles[error.code] ?? "Panel failed to load",
      message: failedQueryText(error),
      retry: true,
      empty: false,
    }
  }
  return {
    title: "Panel failed to load",
    message: error instanceof Error ? error.message : "Unknown error",
    retry: true,
    empty: false,
  }
}

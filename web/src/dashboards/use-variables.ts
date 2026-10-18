import { useState } from "react"
import { keepPreviousData, useQueries } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { Variable } from "@/spec"

import { panelRequestLimiter } from "./limiter"
import { withVariables } from "./search"
import {
  earlierSelections,
  resolveSelections,
  type VariableOptions,
} from "./variables"

export type VariableState = {
  variable: Variable
  options: string[] | undefined
  truncated: boolean
  status: "loading" | "ready" | "error"
  selected: string[]
  retry: () => void
}

type Args = {
  dashboardId: string
  version: number
  variables: Variable[]
  from?: string
  to?: string
  urlVars: Record<string, string[]>
}

export function useVariables({
  dashboardId,
  version,
  variables,
  from,
  to,
  urlVars,
}: Args) {
  const results = useQueries({
    queries: variables.map((variable, index) => {
      const earlier = earlierSelections(variables, index, urlVars)
      return {
        queryKey: dashboardKeys.variableValues(
          dashboardId,
          version,
          variable.name,
          { from, to, selections: earlier }
        ),
        queryFn: ({ signal }: { signal: AbortSignal }) => {
          const range: { from?: string; to?: string } = {}
          if (from !== undefined) range.from = from
          if (to !== undefined) range.to = to
          return panelRequestLimiter.run(
            () =>
              api
                .GET(
                  "/dashboards/{id}/versions/{version}/variables/{name}/values",
                  {
                    params: {
                      path: { id: dashboardId, version, name: variable.name },
                      query: withVariables(range, earlier),
                    },
                    signal,
                  }
                )
                .then(unwrap),
            signal
          )
        },
        enabled: variable.type === "query",
        staleTime: 30_000,
        placeholderData: keepPreviousData,
      }
    }),
  })

  const options: VariableOptions = {}
  variables.forEach((variable, index) => {
    if (variable.type === "custom") {
      options[variable.name] = variable.options
    } else {
      const result = results[index]
      options[variable.name] =
        result.data?.values ?? (result.isError ? [] : undefined)
    }
  })

  const resolved = resolveSelections(variables, urlVars, options)
  const selectionKey = JSON.stringify(resolved.selections)
  const [stable, setStable] = useState({
    key: selectionKey,
    selections: resolved.selections,
  })
  if (stable.key !== selectionKey) {
    setStable({ key: selectionKey, selections: resolved.selections })
  }
  const selections = stable.selections

  const states: VariableState[] = variables.map((variable, index) => {
    const result = results[index]
    const custom = variable.type === "custom"
    return {
      variable,
      options: options[variable.name],
      truncated: !custom && (result.data?.truncated ?? false),
      status: custom
        ? "ready"
        : result.isError
          ? "error"
          : result.data
            ? "ready"
            : "loading",
      selected: selections[variable.name] ?? [],
      retry: () => void result.refetch(),
    }
  })

  return { states, selections, ready: !resolved.pending }
}

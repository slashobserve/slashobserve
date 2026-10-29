import { useState } from "react"
import { useQueries } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { Variable } from "@/spec"

import { panelRequestLimiter } from "./limiter"
import { withVariables } from "./search"
import {
  parentKeyOf,
  resolveSelections,
  valuesRequests,
  variableDisplay,
  type LoadedValues,
  type VariableOptions,
} from "./variables"

export type VariableState = {
  variable: Variable
  options: string[] | undefined
  truncated: boolean
  status: "loading" | "refreshing" | "ready" | "error"
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
  const [knownOptions, setKnownOptions] = useState<VariableOptions>({})
  const requests = valuesRequests(variables, urlVars, knownOptions)
  const results = useQueries({
    queries: variables.map((variable, index) => {
      const { parents: earlier, parentsReady } = requests[index]
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
        enabled:
          parentsReady &&
          (variable.type === "query" || variable.type === "datasource"),
        staleTime: 30_000,
      }
    }),
  })

  const parentKeys = requests.map((request) => parentKeyOf(request.parents))
  const [lastLoaded, setLastLoaded] = useState<Record<string, LoadedValues>>({})
  const scopePrefix = `${dashboardId}:${version}:`
  let nextLoaded = lastLoaded
  if (Object.keys(nextLoaded).some((key) => !key.startsWith(scopePrefix))) {
    nextLoaded = Object.fromEntries(
      Object.entries(nextLoaded).filter(([key]) => key.startsWith(scopePrefix))
    )
  }
  variables.forEach((variable, index) => {
    const data = results[index].data
    const key = `${scopePrefix}${variable.name}`
    const loaded = nextLoaded[key]
    if (
      data &&
      (loaded?.data !== data || loaded.parentKey !== parentKeys[index])
    ) {
      nextLoaded = {
        ...nextLoaded,
        [key]: { data, parentKey: parentKeys[index] },
      }
    }
  })
  if (nextLoaded !== lastLoaded) setLastLoaded(nextLoaded)

  const displays = variables.map((variable, index) =>
    variableDisplay(variable, {
      data: results[index].data,
      last: lastLoaded[`${scopePrefix}${variable.name}`],
      parentKey: parentKeys[index],
      isError: results[index].isError,
      isFetching: results[index].isFetching,
    })
  )
  const selectable: VariableOptions = {}
  variables.forEach((variable, index) => {
    selectable[variable.name] = displays[index].selectable
  })
  const selectableKey = JSON.stringify(selectable)
  if (selectableKey !== JSON.stringify(knownOptions)) {
    setKnownOptions(selectable)
  }
  const resolved = resolveSelections(variables, urlVars, selectable)
  const selectionKey = JSON.stringify(resolved.selections)
  const [stable, setStable] = useState({
    key: selectionKey,
    selections: resolved.selections,
  })
  if (stable.key !== selectionKey) {
    setStable({ key: selectionKey, selections: resolved.selections })
  }
  const selections = stable.selections

  const valueLists: Record<string, string[] | undefined> = {}
  variables.forEach((variable, index) => {
    valueLists[variable.name] = displays[index].shown?.values
  })
  const valuesKey = JSON.stringify(valueLists)
  const [stableValues, setStableValues] = useState({
    key: valuesKey,
    lists: valueLists,
  })
  if (stableValues.key !== valuesKey) {
    setStableValues({ key: valuesKey, lists: valueLists })
  }

  const states: VariableState[] = variables.map((variable, index) => ({
    variable,
    options: displays[index].shown?.values,
    truncated: displays[index].shown?.truncated ?? false,
    status: displays[index].status,
    selected: resolved.selections[variable.name] ?? [],
    retry: () => void results[index].refetch(),
  }))

  const blocking = states.find(
    (state) =>
      state.status === "error" && resolved.waiting.includes(state.variable.name)
  )

  return {
    states,
    selections,
    values: stableValues.lists,
    ready: !resolved.pending,
    blocking,
  }
}

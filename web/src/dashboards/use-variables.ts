import { useState } from "react"
import { useQueries } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { requestZone } from "@/lib/time"
import type { Variable } from "@/spec"

import { panelRequestLimiter } from "./limiter"
import {
  loadRangeScope,
  panelFingerprint,
  sourceScope,
  stateScope,
  variableValuesKey,
  type PanelSource,
} from "./panel-source"
import { useSetDashboardSearch, withVariables } from "./search"
import {
  changeVars,
  mergeVars,
  parentKeyOf,
  resolveSelections,
  valuesRange,
  valuesRequests,
  variableDisplay,
  type LoadedValues,
  type VariableOptions,
  type VariableValues,
} from "./variables"

export type VariableState = {
  variable: Variable
  options: string[] | undefined
  texts: string[] | undefined
  truncated: boolean
  status: "loading" | "refreshing" | "ready" | "error"
  selected: string[]
  retry: () => void
}

type Args = {
  source: PanelSource
  variables: Variable[]
  from?: string
  to?: string
  timezone?: string
  urlVars: Record<string, string[]>
}

export function useVariables({
  source,
  variables,
  from,
  to,
  timezone,
  urlVars,
}: Args) {
  const setSearch = useSetDashboardSearch()
  const owner = stateScope(source)
  const [sessionState, setSessionState] = useState<{
    owner: string
    vars: VariableValues
  }>({ owner, vars: {} })
  const sessionVars = sessionState.owner === owner ? sessionState.vars : {}
  const scope = sourceScope(source)
  const rangeScope = loadRangeScope(source, { from, to })
  const [loadRange, setLoadRange] = useState({ scope: rangeScope, from, to })
  if (loadRange.scope !== rangeScope) {
    setLoadRange({ scope: rangeScope, from, to })
  }
  const atLoad = loadRange.scope === rangeScope ? loadRange : { from, to }
  const vars = mergeVars(variables, { url: urlVars, session: sessionVars })
  const [knownOptions, setKnownOptions] = useState<VariableOptions>({})
  const requests = valuesRequests(variables, vars, knownOptions)
  const results = useQueries({
    queries: variables.map((variable, index) => {
      const { parents: earlier, parentsReady } = requests[index]
      const asked = valuesRange(variable, { from, to }, atLoad)
      return {
        queryKey: variableValuesKey(
          source,
          variable.name,
          source.kind === "build" ? panelFingerprint(variable) : "",
          { from: asked.from, to: asked.to, selections: earlier }
        ),
        queryFn: ({ signal }: { signal: AbortSignal }) => {
          const range: { from?: string; to?: string; tz?: string } = {}
          if (asked.from !== undefined) range.from = asked.from
          if (asked.to !== undefined) range.to = asked.to
          const tz = requestZone(timezone)
          if (tz !== undefined) range.tz = tz
          const params = withVariables(range, earlier)
          return panelRequestLimiter.run(
            () =>
              (source.kind === "version"
                ? api.GET(
                    "/dashboards/{id}/versions/{version}/variables/{name}/values",
                    {
                      params: {
                        path: {
                          id: source.dashboardId,
                          version: source.version,
                          name: variable.name,
                        },
                        query: params,
                      },
                      signal,
                    }
                  )
                : api.GET("/observe/{id}/build/variables/{name}/values", {
                    params: {
                      path: { id: source.requestId, name: variable.name },
                      query: params,
                    },
                    signal,
                  })
              ).then(unwrap),
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
  const scopePrefix = `${scope}:`
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
  const resolved = resolveSelections(variables, vars, selectable)
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
    valueLists[variable.name] = displays[index].selectable?.filter(
      (value) => value !== ""
    )
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
    texts: displays[index].shown?.texts,
    truncated: displays[index].shown?.truncated ?? false,
    status: displays[index].status,
    selected: resolved.selections[variable.name] ?? [],
    retry: () => void results[index].refetch(),
  }))

  const select = (name: string, values: string[]) => {
    const next = changeVars(
      variables,
      { url: urlVars, session: sessionVars },
      name,
      values
    )
    setSessionState({ owner, vars: next.session })
    if (JSON.stringify(next.url) === JSON.stringify(urlVars)) return
    setSearch((current) => ({
      ...current,
      vars: changeVars(
        variables,
        { url: current.vars, session: sessionVars },
        name,
        values
      ).url,
    }))
  }

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
    select,
  }
}

import { useState } from "react"
import { type QueryClient, useQueries } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { requestZone } from "@/lib/time"
import { readBuild } from "@/observe/build"
import type { Variable } from "@/spec"

import { panelRequestLimiter } from "./limiter"
import {
  loadRangeScope,
  panelFingerprint,
  sourceScope,
  stateScope,
  variableValuesKey,
  type VariableSource,
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

type UrlVarsUpdate = (
  current: Record<string, string[]>
) => Record<string, string[]>

type Args = {
  source: VariableSource
  variables: Variable[]
  from?: string | undefined
  to?: string | undefined
  timezone?: string | undefined
  urlVars: Record<string, string[]>
  setUrlVars?: (update: UrlVarsUpdate) => void
}

export function useVariables({
  source,
  variables,
  from,
  to,
  timezone,
  urlVars,
  setUrlVars,
}: Args) {
  const setSearch = useSetDashboardSearch()
  const writeUrlVars =
    setUrlVars ??
    ((update: UrlVarsUpdate) =>
      setSearch((current) => ({ ...current, vars: update(current.vars) })))
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
      const request = requests[index]
      if (request === undefined) {
        throw new Error(`no values request for ${variable.name}`)
      }
      const { parents: earlier, parentsReady } = request
      const asked = valuesRange(variable, { from, to }, atLoad)
      return {
        queryKey: variableValuesKey(
          source,
          variable.name,
          source.kind === "build" ? panelFingerprint(variable) : "",
          { from: asked.from, to: asked.to, selections: earlier }
        ),
        queryFn: ({
          signal,
          client,
        }: {
          signal: AbortSignal
          client: QueryClient
        }) => {
          const range: { from?: string; to?: string; tz?: string } = {}
          if (asked.from !== undefined) range.from = asked.from
          if (asked.to !== undefined) range.to = asked.to
          const tz = requestZone(timezone)
          if (tz !== undefined) range.tz = tz
          const params = withVariables(range, earlier)
          const request = () => {
            switch (source.kind) {
              case "version":
                return api.GET(
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
              case "build":
                return api.GET("/observe/{id}/build/variables/{name}/values", {
                  params: {
                    path: { id: source.requestId, name: variable.name },
                    query: params,
                  },
                  signal,
                })
              case "public":
                return api.GET("/public/{token}/variables/{name}/values", {
                  params: {
                    path: { token: source.token, name: variable.name },
                    query: params,
                  },
                  signal,
                })
            }
          }
          const read = () =>
            panelRequestLimiter.run(() => request().then(unwrap), signal)
          return source.kind === "build"
            ? readBuild(client, source.requestId, signal, read)
            : read()
        },
        enabled:
          parentsReady &&
          (variable.type === "query" || variable.type === "datasource"),
        staleTime: 30_000,
      }
    }),
  })

  const loads = variables.map((variable, index) => {
    const request = requests[index]
    const result = results[index]
    if (request === undefined || result === undefined) {
      throw new Error(`no values query for ${variable.name}`)
    }
    return { variable, result, parentKey: parentKeyOf(request.parents) }
  })
  const [lastLoaded, setLastLoaded] = useState<Record<string, LoadedValues>>({})
  const scopePrefix = `${scope}:`
  let nextLoaded = lastLoaded
  if (Object.keys(nextLoaded).some((key) => !key.startsWith(scopePrefix))) {
    nextLoaded = Object.fromEntries(
      Object.entries(nextLoaded).filter(([key]) => key.startsWith(scopePrefix))
    )
  }
  loads.forEach(({ variable, result, parentKey }) => {
    const data = result.data
    const key = `${scopePrefix}${variable.name}`
    const loaded = nextLoaded[key]
    if (data && (loaded?.data !== data || loaded.parentKey !== parentKey)) {
      nextLoaded = {
        ...nextLoaded,
        [key]: { data, parentKey },
      }
    }
  })
  if (nextLoaded !== lastLoaded) setLastLoaded(nextLoaded)

  const displays = loads.map(({ variable, result, parentKey }) => ({
    variable,
    result,
    display: variableDisplay(variable, {
      data: result.data,
      last: lastLoaded[`${scopePrefix}${variable.name}`],
      parentKey,
      isError: result.isError,
      isFetching: result.isFetching,
    }),
  }))
  const selectable: VariableOptions = {}
  displays.forEach(({ variable, display }) => {
    selectable[variable.name] = display.selectable
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
  displays.forEach(({ variable, display }) => {
    valueLists[variable.name] = display.selectable?.filter(
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

  const states: VariableState[] = displays.map(
    ({ variable, result, display }) => ({
      variable,
      options: display.shown?.values,
      texts: display.shown?.texts,
      truncated: display.shown?.truncated ?? false,
      status: display.status,
      selected: resolved.selections[variable.name] ?? [],
      retry: () => void result.refetch(),
    })
  )

  const select = (name: string, values: string[]) => {
    const next = changeVars(
      variables,
      { url: urlVars, session: sessionVars },
      name,
      values
    )
    setSessionState({ owner, vars: next.session })
    if (JSON.stringify(next.url) === JSON.stringify(urlVars)) return
    writeUrlVars(
      (current) =>
        changeVars(
          variables,
          { url: current, session: sessionVars },
          name,
          values
        ).url
    )
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

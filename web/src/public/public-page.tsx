import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { RefreshCwIcon, TimerIcon } from "lucide-react"

import { publicKeys } from "@/api/keys"
import { Wordmark } from "@/components/brand/wordmark"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { savedCursorSync } from "@/dashboards/cursor-sync"
import { DashboardGrid } from "@/dashboards/dashboard-grid"
import type { PanelSource } from "@/dashboards/panel-source"
import { ALL, type DashboardSearch } from "@/dashboards/search"
import {
  effectiveRefresh,
  pickerQuickRanges,
  pickerRefreshOptions,
} from "@/dashboards/time-picker-settings"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useVariables, type VariableState } from "@/dashboards/use-variables"
import { variableLabel } from "@/dashboards/variable-controls/variable-box"
import { VariablesBar } from "@/dashboards/variables-bar"
import { DatasourceListProvider } from "@/datasources/queries"
import { refreshToMs } from "@/lib/time"
import { createSharedCursor } from "@/panels/shared-cursor"
import { loadProblemText } from "@/service-accounts/queries"
import { ThemeSwitch } from "@/shell/theme-switch"
import { isDataPanel, sourcePanelOf } from "@/spec"

import { PublicPanel } from "./public-panel"
import type { PublicDashboard } from "./queries"
import { viewerMayChoose, viewerVars } from "./supported"

export type SetPublicSearch = (
  update: (state: DashboardSearch) => DashboardSearch,
  options?: { replace?: boolean }
) => void

type Props = {
  token: string
  dashboard: PublicDashboard
  search: DashboardSearch
  onSearchChange: SetPublicSearch
}

const noUnverified = new Map<string, string>()

function isShown(state: VariableState): boolean {
  return (
    state.variable.type !== "constant" && state.variable.hide !== "variable"
  )
}

function valueText(values: string[]): string {
  return values.map((value) => (value === ALL ? "All" : value)).join(", ")
}

export function PublicPage({
  token,
  dashboard,
  search,
  onSearchChange,
}: Props) {
  const { spec, version, variables: mode } = dashboard
  const queryClient = useQueryClient()
  const variables = useMemo(() => spec.variables ?? [], [spec])
  const source = useMemo<PanelSource>(
    () => ({ kind: "public", token, version }),
    [token, version]
  )
  const urlVars = useMemo(
    () => viewerVars(mode, variables, dashboard.selections, search.vars),
    [mode, variables, dashboard.selections, search.vars]
  )
  const setUrlVars = useCallback(
    (update: (current: Record<string, string[]>) => Record<string, string[]>) =>
      onSearchChange((state) => ({ ...state, vars: update(state.vars) })),
    [onSearchChange]
  )
  const { states, selections, values, ready, blocking, select } = useVariables({
    source,
    variables,
    from: search.from,
    to: search.to,
    timezone: spec.timezone,
    urlVars,
    setUrlVars,
  })
  const choosable = states.filter((state) =>
    viewerMayChoose(mode, state.variable)
  )
  const fixed = states.filter(
    (state) => isShown(state) && !viewerMayChoose(mode, state.variable)
  )

  const retryBlocking = useRef<(() => void) | undefined>(undefined)
  useEffect(() => {
    retryBlocking.current = blocking?.retry
  })
  const onRetryBlocked = useCallback(() => retryBlocking.current?.(), [])
  const blockedBy = blocking
    ? (blocking.variable.label ?? blocking.variable.name)
    : undefined

  useEffect(() => {
    const previous = document.title
    document.title = `${dashboard.title} - SlashObserve`
    return () => {
      document.title = previous
    }
  }, [dashboard.title])

  const [cursor] = useState(createSharedCursor)
  useLayoutEffect(() => cursor.setMode(savedCursorSync(spec)), [cursor, spec])

  const effectiveFrom = search.from ?? spec.time.from
  const effectiveTo = search.to ?? spec.time.to
  const refreshValue = effectiveRefresh(search.refresh, spec)
  const refreshMs = refreshToMs(refreshValue)
  const pickerHidden = spec.timePicker?.hidden === true

  const onTimeRangeSelect = useCallback(
    (fromMs: number, toMs: number) =>
      onSearchChange((state) => ({
        ...state,
        from: String(Math.round(fromMs)),
        to: String(Math.round(toMs)),
      })),
    [onSearchChange]
  )

  return (
    <DatasourceListProvider value={dashboard.datasources}>
      <div className="flex min-h-svh flex-col bg-background text-foreground">
        <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 pt-3 pr-2.5 pb-2.5 pl-4">
          <div className="flex min-w-0 items-center gap-2 text-[13px]">
            <Wordmark />
            <span aria-hidden="true" className="text-muted-foreground">
              /
            </span>
            <h1 className="truncate font-medium">{dashboard.title}</h1>
          </div>
          <div className="flex items-center gap-1.5">
            {pickerHidden ? null : (
              <TimeRangeControl
                value={{ from: effectiveFrom, to: effectiveTo }}
                onChange={(next) =>
                  onSearchChange((state) => ({ ...state, ...next }))
                }
                quickRanges={pickerQuickRanges(spec)}
              />
            )}
            <Button
              variant="outline"
              size="icon"
              className="size-[30px]"
              aria-label="Refresh now"
              onClick={() =>
                void queryClient.invalidateQueries({
                  queryKey: publicKeys.dashboard(token),
                })
              }
            >
              <RefreshCwIcon />
            </Button>
            {pickerHidden ? null : (
              <PublicRefreshPicker
                value={refreshValue}
                choices={pickerRefreshOptions(spec)}
                onChange={(refresh) =>
                  onSearchChange((state) => ({ ...state, refresh }), {
                    replace: true,
                  })
                }
              />
            )}
            <ThemeSwitch />
          </div>
        </header>
        <VariablesBar
          states={choosable}
          unverified={noUnverified}
          onSelect={select}
        />
        {fixed.length > 0 ? (
          <p className="flex flex-wrap gap-x-4 gap-y-1 pr-2.5 pb-2.5 pl-4 text-xs text-muted-foreground">
            {fixed.map((state) => (
              <span key={state.variable.name}>
                {variableLabel(state.variable)}: {valueText(state.selected)}
              </span>
            ))}
          </p>
        ) : null}
        <main className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4">
          <DashboardGrid
            dashboardId={dashboard.dashboard_id}
            spec={spec}
            selections={selections}
            values={values}
            renderPanel={(item, size) => (
              <PublicPanel
                source={source}
                cursor={cursor}
                panel={item.panel}
                reused={
                  isDataPanel(item.panel)
                    ? sourcePanelOf(spec, item.panel)
                    : undefined
                }
                variables={variables}
                title={item.title}
                width={size.width}
                height={size.height}
                from={search.from}
                to={search.to}
                timezone={spec.timezone}
                rangeFrom={effectiveFrom}
                rangeTo={effectiveTo}
                selections={item.selections}
                selectionsReady={ready}
                blockedBy={blockedBy}
                onRetryBlocked={onRetryBlocked}
                refreshMs={refreshMs}
                onTimeRangeSelect={onTimeRangeSelect}
              />
            )}
          />
        </main>
      </div>
    </DatasourceListProvider>
  )
}

function PublicRefreshPicker({
  value,
  choices,
  onChange,
}: {
  value: string
  choices: string[]
  onChange: (next: string) => void
}) {
  const items = choices.map((choice) => ({
    value: choice,
    label: choice === "off" ? "Off" : choice,
  }))
  const options = items.some((item) => item.value === value)
    ? items
    : [...items, { value, label: value }]
  const label = options.find((item) => item.value === value)?.label ?? value
  return (
    <Select
      items={options}
      value={value}
      onValueChange={(next) => {
        if (next !== null) onChange(next)
      }}
    >
      <SelectTrigger
        aria-label={`Auto-refresh: ${label}`}
        className="min-w-16 data-[size=default]:h-[30px]"
      >
        <TimerIcon aria-hidden="true" className="text-muted-foreground" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="end">
        {options.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function PublicLinkUnavailable() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <Wordmark />
      <p className="text-sm text-muted-foreground">
        This link does not work. It may have expired or been revoked.
      </p>
    </main>
  )
}

export function PublicLoadError({
  error,
  retrying,
  onRetry,
}: {
  error: unknown
  retrying: boolean
  onRetry: () => void
}) {
  return (
    <main className="mx-auto w-full max-w-lg p-6">
      <LoadProblem
        title="Could not load the dashboard"
        detail={loadProblemText(error)}
        retrying={retrying}
        onRetry={onRetry}
      />
    </main>
  )
}

export function PublicPageSkeleton() {
  return (
    <div
      className="flex flex-col gap-3 pt-3 pr-2.5 pb-2.5 pl-4"
      aria-busy="true"
    >
      <Skeleton className="h-[30px] w-64 animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-64 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}

import { useId, type ReactNode } from "react"
import {
  CircleAlertIcon,
  InfoIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import type { Panel } from "@/spec"

import { VariableLoadError } from "./variables"

export type PanelState = "loading" | "ready" | "empty" | "error"

export const panelHeaderHeight = 32
export const panelBorderWidth = 1

export function panelBodySize(width: number, height: number) {
  return {
    width: Math.max(0, width - 2 * panelBorderWidth),
    height: Math.max(0, height - panelHeaderHeight - 2 * panelBorderWidth),
  }
}

const errorTitles: Record<string, string> = {
  query_timeout: "Query timed out",
  datasource_unavailable: "Datasource unavailable",
  limit_exceeded: "Too much data",
  query_invalid: "Query error",
  query_forbidden: "Query not allowed",
  variable_empty: "Variable has no values",
}

export function errorTitle(error: unknown): string {
  if (error instanceof VariableLoadError) return "Variable failed to load"
  if (error instanceof ApiError && Object.hasOwn(errorTitles, error.code)) {
    return errorTitles[error.code]
  }
  return "Panel failed to load"
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error"
}

type Props = {
  panel: Panel
  state: PanelState
  error?: unknown
  refreshing?: boolean
  staleError?: unknown
  onRetry?: () => void
  menu?: ReactNode
  dragHandle?: boolean
  children?: ReactNode
}

export function PanelChrome({
  panel,
  state,
  error,
  refreshing = false,
  staleError,
  onRetry,
  menu,
  dragHandle = false,
  children,
}: Props) {
  const titleId = useId()
  return (
    <article
      aria-labelledby={titleId}
      aria-busy={state === "loading"}
      data-panel-id={panel.id}
      data-state={state}
      className="flex size-full flex-col overflow-hidden rounded-lg border bg-card text-card-foreground"
    >
      <header
        className={cn(
          "flex shrink-0 items-center gap-1 pr-1 pl-3",
          dragHandle && "panel-drag-handle cursor-grab active:cursor-grabbing"
        )}
        style={{ height: panelHeaderHeight }}
      >
        <h2
          id={titleId}
          title={panel.title}
          className="min-w-0 truncate text-sm font-medium"
        >
          {panel.title}
        </h2>
        {panel.description ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`About ${panel.title}`}
                />
              }
            >
              <InfoIcon />
            </TooltipTrigger>
            <TooltipContent>{panel.description}</TooltipContent>
          </Tooltip>
        ) : null}
        <span className="flex-1" />
        {refreshing ? (
          <span className="flex items-center text-muted-foreground">
            <LoaderCircleIcon className="size-3 motion-safe:animate-spin" />
            <span className="sr-only">Refreshing</span>
          </span>
        ) : null}
        {staleError ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`Showing older data: ${errorMessage(staleError)}`}
                />
              }
            >
              <TriangleAlertIcon className="text-threshold-warning" />
            </TooltipTrigger>
            <TooltipContent>{errorMessage(staleError)}</TooltipContent>
          </Tooltip>
        ) : null}
        {menu}
      </header>
      <div className="min-h-0 flex-1">
        {state === "loading" ? (
          <div className="size-full p-2 pt-0">
            <Skeleton className="size-full animate-none motion-safe:animate-pulse" />
          </div>
        ) : null}
        {state === "empty" ? (
          <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
            No data
          </div>
        ) : null}
        {state === "error" ? (
          <div className="flex size-full flex-col items-center justify-center gap-2 p-3 text-center">
            <CircleAlertIcon className="size-5 text-destructive" />
            <p className="text-sm font-medium">{errorTitle(error)}</p>
            {errorMessage(error).toLowerCase() !==
            errorTitle(error).toLowerCase() ? (
              <p className="line-clamp-3 text-xs text-muted-foreground">
                {errorMessage(error)}
              </p>
            ) : null}
            <Button variant="outline" size="sm" onClick={onRetry}>
              Retry
            </Button>
          </div>
        ) : null}
        {state === "ready" ? children : null}
      </div>
    </article>
  )
}

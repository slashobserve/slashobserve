import { useId, type ReactNode } from "react"
import {
  ClockIcon,
  InfoIcon,
  LinkIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import type { ResolvedLink } from "@/panels/field-config"
import { errorMessage, PanelErrorState } from "@/panels/panel-error"
import { PanelSkeleton } from "@/panels/panel-skeleton"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { anchorProps, NewTabMark } from "./dashboard-links"
import { PanelMoveHandle, PanelResizeGrip } from "./edit/panel-move-handle"
import { timeOverrideLabel } from "./time-override"
import { VariableLoadError } from "./variables"

export type PanelState = "loading" | "ready" | "empty" | "error" | "unsupported"

const panelHeaderHeight = 39
const panelBorderWidth = 1

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

function errorTitle(error: unknown): string {
  if (error instanceof VariableLoadError) return "Variable failed to load"
  if (error instanceof ApiError && Object.hasOwn(errorTitles, error.code)) {
    return errorTitles[error.code]
  }
  return "Panel failed to load"
}

type Props = {
  panel: LayoutPanel
  state: PanelState
  error?: unknown
  refreshing?: boolean
  staleError?: unknown
  onRetry?: () => void
  badge?: ReactNode
  links?: ResolvedLink[]
  menu?: ReactNode
  datasourceLabel?: string
  editing?: boolean
  highlighted?: boolean
  dragHandle?: boolean
  repeatOf?: string
  children?: ReactNode
}

export function PanelChrome({
  panel,
  state,
  error,
  refreshing = false,
  staleError,
  onRetry,
  badge,
  links = [],
  menu,
  datasourceLabel,
  editing = false,
  highlighted = false,
  dragHandle = false,
  repeatOf,
  children,
}: Props) {
  const titleId = useId()
  const title = panelTitle(panel)
  const selected = editing || highlighted
  const timeOverride = isDataPanel(panel) ? timeOverrideLabel(panel) : null
  const titleHidden = isDataPanel(panel) && panel.titleHidden === true
  const transparent = "transparent" in panel && panel.transparent === true
  return (
    <article
      aria-labelledby={titleId}
      aria-busy={state === "loading"}
      data-panel-id={panel.id}
      data-repeat-of={repeatOf}
      data-state={state}
      data-highlighted={selected ? "" : undefined}
      className={cn(
        "relative flex size-full flex-col overflow-hidden rounded-card border text-card-foreground",
        transparent ? "border-transparent bg-transparent" : "bg-card",
        selected &&
          "border-brand outline outline-1 -outline-offset-1 outline-brand"
      )}
    >
      <header
        className={cn(
          "flex shrink-0 items-center gap-2 pt-[9px] pr-2 pb-1",
          dragHandle
            ? "panel-drag-handle cursor-grab pl-1 active:cursor-grabbing"
            : "pl-3"
        )}
        style={{ height: panelHeaderHeight }}
      >
        {dragHandle ? <PanelMoveHandle panel={panel} /> : null}
        <h2
          id={titleId}
          title={title}
          className={cn(
            "min-w-0 truncate text-[13px] font-semibold",
            titleHidden && "sr-only"
          )}
        >
          {title}
        </h2>
        {panel.description ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`About ${title}`}
                />
              }
            >
              <InfoIcon />
            </TooltipTrigger>
            <TooltipContent>{panel.description}</TooltipContent>
          </Tooltip>
        ) : null}
        {datasourceLabel ? (
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {datasourceLabel}
          </span>
        ) : null}
        {timeOverride ? (
          <span
            title={timeOverride.title}
            className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground"
          >
            <ClockIcon aria-hidden="true" className="size-3 shrink-0" />
            <span className="truncate">{timeOverride.text}</span>
          </span>
        ) : null}
        {editing ? (
          <span className="text-xs font-medium text-brand">Editing</span>
        ) : null}
        <div className="ml-auto flex items-center gap-0.5">
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
          {badge}
          {links.length > 0 ? (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    className="text-muted-foreground"
                    aria-label={`Panel links: ${title}`}
                  />
                }
              >
                <LinkIcon aria-hidden="true" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-max">
                {links.map((link, index) => (
                  <DropdownMenuItem
                    key={`${index}:${link.href}`}
                    render={<a {...anchorProps(link)} />}
                    className="justify-between gap-3"
                  >
                    {link.title}
                    {link.newTab ? <NewTabMark /> : null}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
          {menu}
        </div>
      </header>
      <div className="min-h-0 flex-1">
        {state === "loading" ? <PanelSkeleton /> : null}
        {state === "empty" ? (
          <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
            {isDataPanel(panel) ? (panel.noValue ?? "No data") : "No data"}
          </div>
        ) : null}
        {state === "error" ? (
          <PanelErrorState
            title={errorTitle(error)}
            error={error}
            onRetry={onRetry}
          />
        ) : null}
        {state === "unsupported" ? (
          <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
            This panel type is not shown yet.
          </div>
        ) : null}
        {state === "ready" ? children : null}
      </div>
      {dragHandle ? <PanelResizeGrip /> : null}
    </article>
  )
}

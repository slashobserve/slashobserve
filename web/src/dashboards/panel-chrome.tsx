import { useId, type ReactNode } from "react"
import {
  ClockIcon,
  InfoIcon,
  LinkIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react"

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
import { useDraftTitle } from "@/panel-editor/draft-title"
import type { ResolvedLink } from "@/panels/field-config"
import { PanelErrorState } from "@/panels/panel-error"
import { PanelSkeleton } from "@/panels/panel-skeleton"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { anchorProps, NewTabMark } from "./dashboard-links"
import { PanelMoveHandle, PanelMoveHint } from "./edit/panel-move-handle"
import { panelProblem } from "./panel-problem"
import type { SeriesCap } from "./series-cap"
import { timeOverrideLabel } from "./time-override"

export type PanelState = "loading" | "ready" | "empty" | "error" | "unsupported"

const panelHeaderHeight = 39
const panelBorderWidth = 1
export const staleRowHeight = 24

export function panelBodySize(width: number, height: number, stale = false) {
  return {
    width: Math.max(0, width - 2 * panelBorderWidth),
    height: Math.max(
      0,
      height -
        panelHeaderHeight -
        2 * panelBorderWidth -
        (stale ? staleRowHeight : 0)
    ),
  }
}

export function showsStaleRow(state: PanelState, notice: string | undefined) {
  return notice !== undefined && (state === "ready" || state === "empty")
}

function PanelProblemState({
  error,
  onRetry,
}: {
  error: unknown
  onRetry: (() => void) | undefined
}) {
  const problem = panelProblem(error)
  if (problem.empty) {
    return (
      <div className="flex size-full flex-col items-center justify-center gap-1 p-3 text-center">
        <p className="text-sm text-muted-foreground">{problem.title}</p>
        <p className="line-clamp-3 text-xs text-muted-foreground">
          {problem.message}
        </p>
      </div>
    )
  }
  return (
    <PanelErrorState
      title={problem.title}
      message={problem.message}
      onRetry={problem.retry ? onRetry : undefined}
    />
  )
}

type Props = {
  panel: LayoutPanel
  state: PanelState
  error?: unknown
  refreshing?: boolean
  staleNotice?: string
  onRetry?: () => void
  badge?: ReactNode
  links?: ResolvedLink[]
  menu?: ReactNode
  datasourceLabel?: string
  seriesCap?: SeriesCap | null
  editing?: boolean
  highlighted?: boolean
  selected?: boolean
  onSelect?: () => void
  dragHandle?: boolean
  arrangeable?: boolean
  grabbable?: boolean
  repeatOf?: string
  children?: ReactNode
}

export function PanelChrome({
  panel,
  state,
  error,
  refreshing = false,
  staleNotice,
  onRetry,
  badge,
  links = [],
  menu,
  datasourceLabel,
  seriesCap,
  editing = false,
  highlighted = false,
  selected = false,
  onSelect,
  dragHandle = false,
  arrangeable = false,
  grabbable = false,
  repeatOf,
  children,
}: Props) {
  const titleId = useId()
  const draftTitle = useDraftTitle(editing ? panel.id : null)
  const title = draftTitle ?? panelTitle(panel)
  const emphasized = editing || highlighted
  const timeOverride = isDataPanel(panel) ? timeOverrideLabel(panel) : null
  const titleHidden = isDataPanel(panel) && panel.titleHidden === true
  const transparent = "transparent" in panel && panel.transparent === true
  const selectable = onSelect !== undefined && !titleHidden
  const showsStale = showsStaleRow(state, staleNotice)
  return (
    <article
      aria-labelledby={titleId}
      aria-busy={state === "loading"}
      data-panel-id={panel.id}
      data-repeat-of={repeatOf}
      data-state={state}
      data-highlighted={emphasized ? "" : undefined}
      data-selected={selected ? "" : undefined}
      aria-current={selected ? "true" : undefined}
      className={cn(
        "group/panel @container/panel relative flex size-full flex-col overflow-hidden rounded-card border text-card-foreground",
        transparent ? "border-transparent bg-transparent" : "bg-card",
        emphasized &&
          "border-foreground outline outline-1 -outline-offset-1 outline-foreground",
        selected && "ring-2 ring-selection-strong"
      )}
    >
      <header
        className={cn(
          "relative flex shrink-0 items-center gap-2 pt-[9px] pr-2 pb-1 @max-[15rem]/panel:gap-1",
          dragHandle ? "pl-1" : "pl-3",
          dragHandle && "touch-none",
          grabbable &&
            "group/header panel-drag-handle cursor-grab active:cursor-grabbing"
        )}
        title={grabbable ? "Drag to move" : undefined}
        style={{ height: panelHeaderHeight }}
      >
        {dragHandle ? (
          <PanelMoveHandle panel={panel} />
        ) : arrangeable ? (
          <PanelMoveHint shown={grabbable} />
        ) : null}
        <h2
          id={titleId}
          title={title}
          aria-label={selectable ? title : undefined}
          className={cn(
            "min-w-0 text-[13px] font-semibold",
            !selectable && "truncate",
            titleHidden && "sr-only"
          )}
        >
          {selectable ? (
            <button
              type="button"
              data-panel-title=""
              aria-pressed={selected}
              aria-label={`Select panel: ${title}`}
              className={cn(
                "block max-w-full truncate rounded-control text-left font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring",
                grabbable && "cursor-[inherit]"
              )}
              onClick={onSelect}
            >
              {title}
            </button>
          ) : (
            title
          )}
        </h2>
        {datasourceLabel ? (
          <span className="min-w-0 flex-1 basis-0 truncate text-xs text-muted-foreground @max-[15rem]/panel:hidden">
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
        {seriesCap ? (
          <span
            title={seriesCap.title}
            className="min-w-0 truncate text-xs text-muted-foreground"
          >
            {seriesCap.text}
          </span>
        ) : null}
        {editing ? (
          <span className="text-xs font-medium text-foreground">Editing</span>
        ) : null}
        <div
          className="relative ml-auto flex items-center gap-0.5"
          title={grabbable ? "" : undefined}
        >
          {refreshing ? (
            <span className="flex items-center text-muted-foreground">
              <LoaderCircleIcon className="size-3 motion-safe:animate-spin" />
              <span className="sr-only">Refreshing</span>
            </span>
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
          {panel.description ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={`About ${title}`}
                    title={grabbable ? "" : undefined}
                  />
                }
              >
                <InfoIcon />
              </TooltipTrigger>
              <TooltipContent>{panel.description}</TooltipContent>
            </Tooltip>
          ) : null}
          {menu}
        </div>
      </header>
      <div data-panel-body="" className="min-h-0 flex-1">
        {state === "loading" ? <PanelSkeleton /> : null}
        {state === "empty" ? (
          <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
            {isDataPanel(panel) ? (panel.noValue ?? "No data") : "No data"}
          </div>
        ) : null}
        {state === "error" ? (
          <PanelProblemState error={error} onRetry={onRetry} />
        ) : null}
        {state === "unsupported" ? (
          <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
            This panel type is not shown yet.
          </div>
        ) : null}
        {state === "ready" ? children : null}
      </div>
      {showsStale ? (
        <p
          role="status"
          title={staleNotice}
          style={{ height: staleRowHeight }}
          className="flex shrink-0 items-center gap-1.5 border-t px-3 text-xs text-muted-foreground"
        >
          <TriangleAlertIcon
            aria-hidden="true"
            className="size-3 shrink-0 text-threshold-warning"
          />
          <span className="truncate">{staleNotice}</span>
        </p>
      ) : null}
    </article>
  )
}

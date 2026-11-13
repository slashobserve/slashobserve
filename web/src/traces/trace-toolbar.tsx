import type { KeyboardEvent, ReactNode, Ref } from "react"
import { ChevronDownIcon, ChevronUpIcon, SearchIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { styleForSlot } from "@/panels/colors"

export type TraceService = {
  name: string
  slot: number
  spans: number
  selfMs: number
}

type TraceToolbarProps = {
  search: string
  onSearchChange: (text: string) => void
  searchRef?: Ref<HTMLInputElement>
  matchIndex: number | null
  matchCount: number
  criticalPath: boolean
  onCriticalPathChange: (on: boolean) => void
  criticalPathDisabled?: string
  errorsOnly: boolean
  onErrorsOnlyChange: (on: boolean) => void
  onCollapseAll: () => void
  onExpandAll: () => void
  onPrevMatch: () => void
  onNextMatch: () => void
}

type ServiceLegendProps = {
  services: TraceService[]
  hiddenServices: ReadonlySet<string>
  onToggleService: (name: string) => void
  hint: string
  formatDuration: (ms: number) => string
}

const pressedClass = "aria-pressed:border-foreground aria-pressed:bg-muted"

export function highlightText(text: string, needle: string): ReactNode {
  const wanted = needle.toLowerCase()
  if (wanted === "" || !text.toLowerCase().includes(wanted)) return text
  const parts: ReactNode[] = []
  const lower = text.toLowerCase()
  let from = 0
  let at = lower.indexOf(wanted)
  while (at !== -1) {
    if (at > from) parts.push(text.slice(from, at))
    parts.push(
      <mark key={at} className="rounded-[2px] bg-status-warning text-[#0a0a0a]">
        {text.slice(at, at + wanted.length)}
      </mark>
    )
    from = at + wanted.length
    at = lower.indexOf(wanted, from)
  }
  if (from < text.length) parts.push(text.slice(from))
  return parts
}

function ToggleButton({
  pressed,
  onChange,
  children,
}: {
  pressed: boolean
  onChange: (on: boolean) => void
  children: ReactNode
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-pressed={pressed}
      className={pressedClass}
      onClick={() => onChange(!pressed)}
    >
      {children}
    </Button>
  )
}

export function TraceToolbar({ searchRef, ...props }: TraceToolbarProps) {
  const matchText =
    props.matchCount === 0 || props.matchIndex === null
      ? `0 of ${props.matchCount}`
      : `${props.matchIndex + 1} of ${props.matchCount}`

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter" || props.matchCount === 0) return
    event.preventDefault()
    if (event.shiftKey) props.onPrevMatch()
    else props.onNextMatch()
  }

  const criticalPathButton = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-pressed={props.criticalPath}
      disabled={props.criticalPathDisabled !== undefined}
      className={pressedClass}
      onClick={() => props.onCriticalPathChange(!props.criticalPath)}
    >
      Critical path
    </Button>
  )

  return (
    <div
      role="toolbar"
      aria-label="Trace tools"
      className="flex flex-wrap items-center gap-2 text-[13px]"
    >
      <div className="flex min-w-0 items-center gap-1">
        <div className="relative w-60 max-w-full">
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            ref={searchRef}
            type="search"
            value={props.search}
            placeholder="Find in trace"
            aria-label="Find spans by name, service or attribute value"
            className="pl-8"
            onChange={(event) => props.onSearchChange(event.target.value)}
            onKeyDown={onSearchKeyDown}
          />
        </div>
        {props.search === "" ? null : (
          <>
            <span
              role="status"
              className="shrink-0 text-xs text-muted-foreground tabular-nums"
            >
              {matchText}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Previous match"
              disabled={props.matchCount === 0}
              onClick={props.onPrevMatch}
            >
              <ChevronUpIcon aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Next match"
              disabled={props.matchCount === 0}
              onClick={props.onNextMatch}
            >
              <ChevronDownIcon aria-hidden="true" />
            </Button>
          </>
        )}
      </div>
      {props.criticalPathDisabled === undefined ? (
        criticalPathButton
      ) : (
        <Tooltip>
          <TooltipTrigger render={<span className="inline-flex" />}>
            {criticalPathButton}
          </TooltipTrigger>
          <TooltipContent>{props.criticalPathDisabled}</TooltipContent>
        </Tooltip>
      )}
      <ToggleButton
        pressed={props.errorsOnly}
        onChange={props.onErrorsOnlyChange}
      >
        Errors only
      </ToggleButton>
      <span className="ml-auto flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={props.onCollapseAll}
        >
          Collapse all
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={props.onExpandAll}
        >
          Expand all
        </Button>
      </span>
    </div>
  )
}

export function ServiceLegend(props: ServiceLegendProps) {
  return (
    <div className="flex h-11 shrink-0 items-center gap-2 overflow-x-auto px-4">
      <span className="shrink-0 text-xs text-muted-foreground">Services</span>
      {props.services.map((service) => {
        const shown = !props.hiddenServices.has(service.name)
        return (
          <Button
            key={service.name}
            type="button"
            variant="outline"
            size="xs"
            aria-pressed={shown}
            title={`${service.name}: ${props.formatDuration(service.selfMs)} of own time`}
            className={cn("gap-1.5", !shown && "opacity-40")}
            onClick={() => props.onToggleService(service.name)}
          >
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-[2px]"
              style={{ backgroundColor: styleForSlot(service.slot).color }}
            />
            {service.name}
            <span className="text-muted-foreground tabular-nums">
              {service.spans.toLocaleString("en")}
            </span>
          </Button>
        )
      })}
      <span className="ml-auto shrink-0 pl-2 text-xs text-muted-foreground">
        {props.hint}
      </span>
    </div>
  )
}

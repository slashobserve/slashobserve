import { useRef, useState } from "react"
import { Link } from "@tanstack/react-router"
import { ChevronDownIcon } from "lucide-react"

import { StatusIcon } from "@/components/status-label"
import {
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuLinkItem,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import { logsLink, tracesLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import type { LogsSearch } from "@/logs/search"
import { railFlyoutItemClass, railFlyoutLabelClass } from "@/shell/rail-flyout"
import type { TraceSearch } from "@/traces/search"

import type { DatasourceStatus } from "./queries"
import { sourceLanguage, type ExploreKind } from "./sources"
import {
  useExploreSources,
  useRouteSource,
  useSourceSwitchSearch,
} from "./use-sources"

const kindWord = { logs: "log", traces: "trace" } as const

function sourceTitle(kind: ExploreKind, source: DatasourceStatus): string {
  const base = `${source.name}, ${sourceLanguage(kind, source.type)}`
  if (source.healthy) return base
  return `${base}, unavailable${source.message ? `: ${source.message}` : ""}`
}

function SourceRowContent({
  kind,
  source,
}: {
  kind: ExploreKind
  source: DatasourceStatus
}) {
  return (
    <>
      <span className="min-w-0 truncate">{source.name}</span>
      <span className="flex flex-none items-center gap-1 text-xs text-muted-foreground">
        {source.healthy ? null : (
          <>
            <StatusIcon tone="critical" />
            <span className="sr-only">, unavailable</span>
          </>
        )}
        {sourceLanguage(kind, source.type)}
      </span>
    </>
  )
}

function useSourceTarget(kind: ExploreKind, source: DatasourceStatus) {
  const search = useSourceSwitchSearch(kind, source.type)
  return kind === "logs"
    ? logsLink(source.uid, search as LogsSearch)
    : tracesLink(source.uid, search as TraceSearch)
}

export function SourceLink({
  kind,
  source,
  current,
  className,
  onNavigate,
}: {
  kind: ExploreKind
  source: DatasourceStatus
  current: boolean
  className?: string
  onNavigate?: () => void
}) {
  const target = useSourceTarget(kind, source)
  return (
    <Link
      {...target}
      onClick={onNavigate}
      title={sourceTitle(kind, source)}
      aria-current={current ? "page" : undefined}
      className={className}
    >
      <SourceRowContent kind={kind} source={source} />
    </Link>
  )
}

function SourceFlyoutItem({
  kind,
  source,
}: {
  kind: ExploreKind
  source: DatasourceStatus
}) {
  const target = useSourceTarget(kind, source)
  const { current } = useRouteSource(kind)
  return (
    <DropdownMenuLinkItem
      render={<Link {...target} />}
      title={sourceTitle(kind, source)}
      aria-current={current?.uid === source.uid ? "page" : undefined}
      className={railFlyoutItemClass}
    >
      <SourceRowContent kind={kind} source={source} />
    </DropdownMenuLinkItem>
  )
}

export function SourceFlyoutItems({ kind }: { kind: ExploreKind }) {
  const { sources } = useExploreSources(kind)
  return (
    <>
      <DropdownMenuLinkItem
        render={<Link to={kind === "logs" ? "/logs" : "/traces"} />}
        className={cn(railFlyoutItemClass, "font-semibold")}
      >
        {kind === "logs" ? "Logs" : "Traces"}
      </DropdownMenuLinkItem>
      <DropdownMenuGroup>
        <DropdownMenuLabel className={railFlyoutLabelClass}>
          {kind === "logs" ? "Log sources" : "Trace sources"}
        </DropdownMenuLabel>
        <div className="max-h-80 overflow-y-auto">
          {sources.map((source) => (
            <SourceFlyoutItem key={source.uid} kind={kind} source={source} />
          ))}
        </div>
      </DropdownMenuGroup>
    </>
  )
}

function SourceSheetList({
  kind,
  onNavigate,
}: {
  kind: ExploreKind
  onNavigate: () => void
}) {
  const { current, sources } = useRouteSource(kind)
  return (
    <ul
      aria-label={kind === "logs" ? "Log sources" : "Trace sources"}
      className="flex max-h-[60svh] flex-col gap-0.5 overflow-y-auto px-2 pb-3"
    >
      {sources.map((source) => (
        <li key={source.uid}>
          <SourceLink
            kind={kind}
            source={source}
            current={current?.uid === source.uid}
            onNavigate={onNavigate}
            className="flex h-11 items-center justify-between gap-2 rounded-control px-3 text-[15px] text-foreground no-underline outline-none hover:bg-nav-active focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-nav-active"
          />
        </li>
      ))}
    </ul>
  )
}

const phoneTitleClass = "truncate text-base leading-5 font-semibold"

export function PhoneSourceTitle({
  kind,
  label,
  source,
}: {
  kind: ExploreKind
  label: string
  source: DatasourceStatus
}) {
  const { sources } = useExploreSources(kind)
  const [open, setOpen] = useState(false)
  const switcher = useRef<HTMLButtonElement>(null)
  const language = sourceLanguage(kind, source.type)
  const lines = (
    <>
      <span className={phoneTitleClass}>{label}</span>
      <span className="flex items-center gap-1 text-xs leading-4 text-muted-foreground">
        <span className="truncate">{`${source.name} · ${language}`}</span>
        {sources.length > 1 ? (
          <ChevronDownIcon aria-hidden="true" className="size-3 flex-none" />
        ) : null}
      </span>
    </>
  )
  if (sources.length < 2) {
    return <div className="flex min-w-0 flex-col">{lines}</div>
  }
  return (
    <>
      <button
        ref={switcher}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={`${label} in ${source.name}, ${language}. Switch ${kindWord[kind]} source`}
        onClick={() => setOpen(true)}
        className="flex h-11 min-w-0 flex-col items-start justify-center rounded-control px-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {lines}
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          side="bottom"
          finalFocus={switcher}
          className="gap-1 pt-3"
        >
          <SheetTitle className="px-4">
            {kind === "logs" ? "Log sources" : "Trace sources"}
          </SheetTitle>
          <SourceSheetList kind={kind} onNavigate={() => setOpen(false)} />
        </SheetContent>
      </Sheet>
    </>
  )
}

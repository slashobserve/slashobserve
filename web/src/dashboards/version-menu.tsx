import { useEffect, useId, useRef } from "react"
import { Link } from "@tanstack/react-router"
import { ArrowRightIcon, ChevronDownIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { RelativeTime } from "@/components/relative-time"
import { Skeleton } from "@/components/ui/skeleton"
import { useVersionHistory } from "@/history/use-version-history"
import { versionActor } from "@/lib/origin"
import { cn } from "@/lib/utils"

import type { LiveStatus } from "./live/use-dashboard-events"
import { useSetDashboardSearch } from "./search"
import {
  isPinnedOlder,
  versionButtonLabel,
  versionButtonText,
} from "./version-label"

const listedVersions = 5

// Pinning or unpinning swaps the page component, so this menu remounts and its trigger loses the focus Base UI would return to it.
let restoreFocusOnMount = false

type Props = {
  dashboardId: string
  version: number
  latest: number
  pinned: boolean
  status: LiveStatus
  back: string | undefined
}

function VersionRows({
  history,
  latest,
}: {
  history: ReturnType<typeof useVersionHistory>
  latest: number
}) {
  if (history.isPending) {
    return (
      <div className="flex flex-col gap-2 px-3 py-2">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
      </div>
    )
  }
  if (history.isError) {
    return (
      <p className="px-3 py-2 text-xs text-muted-foreground">
        Could not load versions.
      </p>
    )
  }
  return history.data.pages[0].versions.slice(0, listedVersions).map((item) => (
    <DropdownMenuRadioItem
      key={item.version}
      value={String(item.version)}
      closeOnClick
      className="items-start gap-2.5 rounded-none px-3 py-2 pr-3 data-checked:bg-muted [&_[data-slot=dropdown-menu-radio-item-indicator]]:hidden"
    >
      <span className="w-6 flex-none text-[13px] font-semibold">
        v{item.version}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-px">
        <span className="text-[13px]">{item.summary || "No summary"}</span>
        <span className="text-xs text-muted-foreground">
          {versionActor(item).who} · <RelativeTime iso={item.created_at} />
        </span>
      </span>
      {item.version === latest ? (
        <span className="text-xs text-muted-foreground">Latest</span>
      ) : null}
    </DropdownMenuRadioItem>
  ))
}

export function VersionMenu({
  dashboardId,
  version,
  latest,
  pinned,
  status,
  back,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const history = useVersionHistory(dashboardId)
  const button = useRef<HTMLButtonElement>(null)
  const headingId = useId()
  const state = { version, latest, pinned, status }

  useEffect(() => {
    if (!restoreFocusOnMount) return
    restoreFocusOnMount = false
    button.current?.focus()
  }, [version])

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              ref={button}
              type="button"
              aria-label={versionButtonLabel(state)}
              className="ml-0.5 flex h-[26px] items-center gap-1.5 rounded-control border border-transparent px-2 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-expanded:border-border"
            />
          }
        >
          <span
            aria-hidden="true"
            className={cn(
              "size-1.5 rounded-full",
              isPinnedOlder(state)
                ? "bg-muted-foreground"
                : status === "live"
                  ? "bg-status-ok"
                  : "bg-muted-foreground motion-safe:animate-pulse"
            )}
          />
          {versionButtonText(state)}
          <ChevronDownIcon aria-hidden="true" className="size-3" />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          sideOffset={6}
          aria-labelledby={headingId}
          className="w-[340px] max-w-[calc(100vw-32px)] rounded-[8px] p-0"
        >
          <div className="flex items-center justify-between px-3 pt-2.5 pb-1.5 text-xs text-muted-foreground">
            <span id={headingId}>Versions</span>
            <span>{latest} saved</span>
          </div>
          <DropdownMenuRadioGroup
            value={String(version)}
            onValueChange={(value) => {
              const next = Number(value)
              restoreFocusOnMount = true
              setSearch((state) => ({
                ...state,
                version: next === latest ? undefined : next,
              }))
            }}
          >
            <VersionRows history={history} latest={latest} />
          </DropdownMenuRadioGroup>
          <DropdownMenuItem
            render={
              <Link
                to="/dashboards/$dashboardId/versions"
                params={{ dashboardId }}
                search={{ back }}
              />
            }
            className="mt-1 justify-between rounded-none border-t px-3 py-2.5 text-[13px]"
          >
            Compare versions
            <ArrowRightIcon aria-hidden="true" className="size-3.5" />
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <span role="status" className="sr-only">
        {status === "reconnecting" ? "Reconnecting to live updates" : ""}
      </span>
    </>
  )
}

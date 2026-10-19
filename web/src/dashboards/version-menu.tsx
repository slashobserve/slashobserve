import { useEffect, useId, useRef, useState } from "react"
import { Link, useLocation, useRouter } from "@tanstack/react-router"
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
import {
  useVersionHistory,
  type VersionMeta,
} from "@/history/use-version-history"
import { versionActor } from "@/lib/origin"
import { cn } from "@/lib/utils"

import type { LiveStatus } from "./live/use-dashboard-events"
import { useSetDashboardSearch } from "./search"
import {
  isOlderVersion,
  versionButtonLabel,
  versionButtonText,
} from "./version-label"

const listedVersions = 5
const reconnectingText = "Reconnecting to live updates"

type Props = {
  dashboardId: string
  version: number
  latest: number
  current: VersionMeta
  pinned: boolean
  editing: boolean
  status: LiveStatus
  back: string | undefined
}

function listedRows(
  history: ReturnType<typeof useVersionHistory>,
  version: number,
  current: VersionMeta
): VersionMeta[] | undefined {
  if (history.data === undefined) return undefined
  const newest = history.data.pages[0].versions.slice(0, listedVersions)
  if (newest.some((item) => item.version === version)) return newest
  const shown =
    history.data.pages
      .flatMap((page) => page.versions)
      .find((item) => item.version === version) ??
    (current.version === version ? current : undefined)
  return shown ? [...newest, shown] : newest
}

function VersionRows({
  rows,
  failed,
  latest,
}: {
  rows: VersionMeta[] | undefined
  failed: boolean
  latest: number
}) {
  if (rows === undefined) {
    if (failed) {
      return (
        <p className="px-3 py-2 text-xs text-muted-foreground">
          Could not load versions.
        </p>
      )
    }
    return (
      <div className="flex flex-col gap-2 px-3 py-2">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
      </div>
    )
  }
  return rows.map((item) => (
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
  latest: dashboardLatest,
  current,
  pinned,
  editing,
  status,
  back,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const router = useRouter()
  const history = useVersionHistory(dashboardId)
  const button = useRef<HTMLButtonElement>(null)
  const headingId = useId()
  const reasonId = useId()
  const latest = Math.max(
    dashboardLatest,
    history.data?.pages[0]?.versions[0]?.version ?? 0
  )
  const state = { version, latest, status }

  const [seenStatus, setSeenStatus] = useState(status)
  const [announcement, setAnnouncement] = useState("")
  if (status !== seenStatus) {
    setSeenStatus(status)
    if (status === "reconnecting") {
      setAnnouncement(reconnectingText)
    } else if (status === "live" && announcement === reconnectingText) {
      setAnnouncement("Live updates restored")
    }
  }

  // Pinning or unpinning swaps the page component, so this menu remounts and its trigger loses the focus Base UI would return to it.
  // The pick carries the version it pins (0 for Live) in its history entry, and the menu that shows that version focuses the button and clears the entry.
  const shown = pinned ? version : 0
  const focusAt = useLocation({
    select: (location) => location.state.focusVersionButtonAt,
  })
  useEffect(() => {
    if (focusAt !== shown) return
    button.current?.focus()
    const { publicHref, state: historyState } = router.state.location
    router.history.replace(
      publicHref,
      { ...historyState, focusVersionButtonAt: undefined },
      { ignoreBlocker: true }
    )
  }, [focusAt, shown, router])

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
              isOlderVersion(state)
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
          sideOffset={4}
          aria-labelledby={headingId}
          className="w-[340px] max-w-[calc(100vw-32px)] rounded-[8px] p-0"
        >
          <div className="flex items-center justify-between px-3 pt-2.5 pb-1.5 text-xs text-muted-foreground">
            <span id={headingId}>Versions</span>
            <span>{latest} saved</span>
          </div>
          {editing ? (
            <p
              id={reasonId}
              className="px-3 pb-1.5 text-xs text-muted-foreground"
            >
              Save or cancel your layout changes to switch versions.
            </p>
          ) : null}
          <DropdownMenuRadioGroup
            value={String(version)}
            disabled={editing}
            aria-describedby={editing ? reasonId : undefined}
            onValueChange={(value) => {
              const next = Number(value) === latest ? 0 : Number(value)
              if (next === shown) return
              setSearch((state) => ({ ...state, version: next || undefined }), {
                state: { focusVersionButtonAt: next },
              })
            }}
          >
            <VersionRows
              rows={listedRows(history, version, current)}
              failed={history.isError}
              latest={latest}
            />
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
        {announcement}
      </span>
    </>
  )
}

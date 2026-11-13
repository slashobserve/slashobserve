import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
} from "react"
import { Autocomplete } from "@base-ui/react/autocomplete"
import { Dialog } from "@base-ui/react/dialog"
import { useQuery } from "@tanstack/react-query"
import { useNavigate, useRouteContext, useRouter } from "@tanstack/react-router"
import {
  ArrowUpRightIcon,
  BookmarkIcon,
  BotIcon,
  ChartGanttIcon,
  DatabaseIcon,
  FlagIcon,
  InboxIcon,
  KeyIcon,
  LayoutDashboardIcon,
  LogsIcon,
  Loader2Icon,
  LogOutIcon,
  MessageSquareIcon,
  NotebookTextIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  SunMoonIcon,
  TerminalIcon,
  UploadIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { observationsLink } from "@/lib/links"
import { isModChord } from "@/lib/shortcuts"
import { useDebouncedValue } from "@/lib/use-debounced-value"

import {
  minSearchLength,
  paletteGroups,
  type PagePath,
  type MatchSegment,
  type PaletteEntry,
  type ResultEntry,
} from "./palette-items"
import { dashboardTitlesQuery, workspaceSearchQuery } from "./palette-queries"
import { usePaletteStore } from "./palette-store"
import { useToggleTheme } from "./theme-switch"
import { useSignOut } from "./use-sign-out"

const pageIcons: Record<PagePath, LucideIcon> = {
  "/inbox": InboxIcon,
  "/observations": NotebookTextIcon,
  "/dashboards": LayoutDashboardIcon,
  "/logs": LogsIcon,
  "/traces": ChartGanttIcon,
  "/agents": BotIcon,
  "/datasources": DatabaseIcon,
  "/keys": KeyIcon,
  "/people": UsersIcon,
  "/settings": SettingsIcon,
}

const resultIcons: Record<ResultEntry["kind"], LucideIcon> = {
  dashboard: LayoutDashboardIcon,
  comment: MessageSquareIcon,
  annotation: FlagIcon,
  observation: NotebookTextIcon,
  saved_search: BookmarkIcon,
}

const actionIcons: Record<
  Extract<PaletteEntry, { kind: "action" }>["id"],
  LucideIcon
> = {
  "action:new-observation": PlusIcon,
  "action:ask": TerminalIcon,
  "action:import": UploadIcon,
  "action:theme": SunMoonIcon,
  "action:sign-out": LogOutIcon,
}

const iconClass = "size-4 flex-none text-muted-foreground"
const searchDebounceMs = 150

function EntryIcon({ entry }: { entry: PaletteEntry }) {
  if (entry.kind === "page") {
    const Icon = pageIcons[entry.to]
    return <Icon aria-hidden="true" className={iconClass} strokeWidth={1.8} />
  }
  if (entry.kind === "action") {
    const Icon = actionIcons[entry.id]
    return <Icon aria-hidden="true" className={iconClass} strokeWidth={1.8} />
  }
  const Icon = resultIcons[entry.kind]
  return <Icon aria-hidden="true" className={iconClass} strokeWidth={1.8} />
}

function isResult(entry: PaletteEntry | undefined): entry is ResultEntry {
  return entry !== undefined && entry.kind !== "page" && entry.kind !== "action"
}

function Segments({ segments }: { segments: MatchSegment[] }) {
  return (
    <>
      {segments.map((segment, index) =>
        segment.match ? (
          <mark
            key={index}
            className="bg-transparent font-medium text-foreground"
          >
            {segment.text}
          </mark>
        ) : (
          <span key={index}>{segment.text}</span>
        )
      )}
    </>
  )
}

function PaletteBody({ close }: { close: () => void }) {
  const navigate = useNavigate()
  const router = useRouter()
  const { principal } = useRouteContext({ from: "/_authed" })
  const [query, setQuery] = useState("")
  const { signOut } = useSignOut()
  const toggleTheme = useToggleTheme()
  const titles = useQuery(dashboardTitlesQuery())
  const needle = query.trim()
  const settled = useDebouncedValue(needle, needle, searchDebounceMs)
  const searching = needle.length >= minSearchLength
  const search = useQuery({
    ...workspaceSearchQuery(settled),
    enabled: settled.length >= minSearchLength,
  })
  const searchPending =
    searching && !search.isError && (settled !== needle || search.isFetching)
  const groups = paletteGroups(
    query,
    titles.data ?? [],
    search.data ?? [],
    principal.role
  )

  const highlighted = useRef<PaletteEntry | undefined>(undefined)

  function openInNewTab(entry: ResultEntry) {
    close()
    window.open(entry.path, "_blank", "noopener")
  }

  function select(entry: PaletteEntry, event: ReactMouseEvent) {
    if (isResult(entry) && (event.metaKey || event.ctrlKey)) {
      openInNewTab(entry)
      return
    }
    close()
    if (entry.kind === "page") {
      void navigate({ to: entry.to })
    } else if (isResult(entry)) {
      router.history.push(entry.path)
    } else if (entry.id === "action:new-observation") {
      void navigate(observationsLink({ new: "1" }))
    } else if (entry.id === "action:ask") {
      void navigate({ to: "/ask" })
    } else if (entry.id === "action:import") {
      void navigate({ to: "/import" })
    } else if (entry.id === "action:theme") {
      toggleTheme()
    } else {
      void signOut()
    }
  }

  function openHighlightedInNewTab(event: ReactKeyboardEvent) {
    const entry = highlighted.current
    if (
      event.key !== "Enter" ||
      !(event.metaKey || event.ctrlKey) ||
      !isResult(entry)
    ) {
      return
    }
    event.preventDefault()
    openInNewTab(entry)
  }

  return (
    <>
      <Autocomplete.Root
        inline
        open
        items={groups}
        filteredItems={groups}
        value={query}
        onValueChange={setQuery}
        autoHighlight="always"
        onItemHighlighted={(entry) => {
          highlighted.current = entry
        }}
        itemToStringValue={(entry: PaletteEntry) => entry.label}
      >
        <div className="flex h-11 items-center gap-2 border-b px-3">
          <SearchIcon
            aria-hidden="true"
            className="size-4 flex-none text-muted-foreground"
            strokeWidth={1.8}
          />
          <Autocomplete.Input
            aria-label="Search the workspace or jump to a page"
            placeholder="Search the workspace or jump to a page"
            onKeyDown={openHighlightedInNewTab}
            className="h-full min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>
        <Autocomplete.List className="max-h-[min(360px,60vh)] overflow-y-auto p-1">
          {(group: (typeof groups)[number]) => (
            <Autocomplete.Group key={group.value} items={group.items}>
              <Autocomplete.GroupLabel className="px-2.5 pt-2 pb-1 text-xs text-muted-foreground">
                {group.value}
              </Autocomplete.GroupLabel>
              <Autocomplete.Collection>
                {(entry: PaletteEntry) => (
                  <Autocomplete.Item
                    key={entry.id}
                    value={entry}
                    onClick={(event) => select(entry, event)}
                    className="group/item flex h-9 cursor-default items-center gap-2.5 rounded-control px-2.5 text-[13px] data-highlighted:bg-selection"
                  >
                    <EntryIcon entry={entry} />
                    <span className="max-w-[60%] flex-none truncate">
                      {entry.label}
                    </span>
                    {isResult(entry) && entry.secondary.length > 0 ? (
                      <span className="min-w-0 truncate text-xs text-muted-foreground">
                        <Segments segments={entry.secondary} />
                      </span>
                    ) : null}
                    {isResult(entry) ? (
                      <button
                        type="button"
                        tabIndex={-1}
                        aria-label={`Open ${entry.label} in a new tab`}
                        title={`Open ${entry.label} in a new tab`}
                        onClick={(event) => {
                          event.stopPropagation()
                          openInNewTab(entry)
                        }}
                        className="ml-auto flex-none rounded-sm p-0.5 text-muted-foreground opacity-0 outline-none group-hover/item:opacity-100 group-data-highlighted/item:opacity-100 hover:text-foreground"
                      >
                        <ArrowUpRightIcon
                          aria-hidden="true"
                          className="size-3.5"
                        />
                      </button>
                    ) : null}
                  </Autocomplete.Item>
                )}
              </Autocomplete.Collection>
            </Autocomplete.Group>
          )}
        </Autocomplete.List>
        <Autocomplete.Empty className="px-2.5 py-6 text-center text-[13px] text-muted-foreground empty:hidden">
          {searchPending ? null : <>Nothing matches &quot;{needle}&quot;.</>}
        </Autocomplete.Empty>
      </Autocomplete.Root>
      {titles.isPending ? (
        <div
          role="status"
          className="flex items-center gap-2 border-t px-2.5 py-2 text-[13px] text-muted-foreground"
        >
          <Loader2Icon
            aria-hidden="true"
            className="size-3.5 motion-safe:animate-spin"
          />
          Loading dashboards
        </div>
      ) : null}
      {searchPending ? (
        <div
          role="status"
          className="flex items-center gap-2 border-t px-2.5 py-2 text-[13px] text-muted-foreground"
        >
          <Loader2Icon
            aria-hidden="true"
            className="size-3.5 motion-safe:animate-spin"
          />
          Searching
        </div>
      ) : null}
      {search.isError ? (
        <div className="flex items-center justify-between gap-2 border-t px-2.5 py-2 text-[13px] text-muted-foreground">
          <span>Could not search.</span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void search.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : null}
      {titles.isError ? (
        <div className="flex items-center justify-between gap-2 border-t px-2.5 py-2 text-[13px] text-muted-foreground">
          <span>Could not load dashboards.</span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void titles.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : null}
    </>
  )
}

export function CommandPalette() {
  const open = usePaletteStore((state) => state.open)
  const setOpen = usePaletteStore((state) => state.setOpen)
  const toggle = usePaletteStore((state) => state.toggle)

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (!isModChord(event, "k")) return
      event.preventDefault()
      toggle()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [toggle])

  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-black/10" />
        <Dialog.Popup className="fixed top-[12vh] left-1/2 z-50 w-[min(560px,calc(100vw-2rem))] -translate-x-1/2 overflow-hidden rounded-card border bg-popover text-popover-foreground outline-none">
          <Dialog.Title className="sr-only">Search or jump to</Dialog.Title>
          <PaletteBody close={() => setOpen(false)} />
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

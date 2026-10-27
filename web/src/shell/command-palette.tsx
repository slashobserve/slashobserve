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
import { useNavigate } from "@tanstack/react-router"
import {
  ArrowUpRightIcon,
  DatabaseIcon,
  KeyIcon,
  LayoutDashboardIcon,
  Loader2Icon,
  LogOutIcon,
  SearchIcon,
  SettingsIcon,
  TerminalIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"

import type { NavPath } from "./nav"
import {
  filterPalette,
  isPaletteShortcut,
  type DashboardEntry,
  type PaletteEntry,
} from "./palette-items"
import { dashboardTitlesQuery } from "./palette-queries"
import { usePaletteStore } from "./palette-store"
import { useSignOut } from "./use-sign-out"

const pageIcons: Record<NavPath, LucideIcon> = {
  "/": TerminalIcon,
  "/dashboards": LayoutDashboardIcon,
  "/datasources": DatabaseIcon,
  "/keys": KeyIcon,
  "/people": UsersIcon,
  "/settings": SettingsIcon,
}

const iconClass = "size-4 flex-none text-muted-foreground"

function EntryIcon({ entry }: { entry: PaletteEntry }) {
  if (entry.kind === "page") {
    const Icon = pageIcons[entry.to]
    return <Icon aria-hidden="true" className={iconClass} strokeWidth={1.8} />
  }
  if (entry.kind === "action") {
    return (
      <LogOutIcon aria-hidden="true" className={iconClass} strokeWidth={1.8} />
    )
  }
  return (
    <LayoutDashboardIcon
      aria-hidden="true"
      className={iconClass}
      strokeWidth={1.8}
    />
  )
}

function PaletteBody({ close }: { close: () => void }) {
  const navigate = useNavigate()
  const [query, setQuery] = useState("")
  const { signOut } = useSignOut()
  const titles = useQuery(dashboardTitlesQuery())
  const groups = filterPalette(query, titles.data ?? [])

  const highlighted = useRef<PaletteEntry | undefined>(undefined)

  function openInNewTab(entry: DashboardEntry) {
    close()
    window.open(`/dashboards/${entry.id}`, "_blank", "noopener")
  }

  function select(entry: PaletteEntry, event: ReactMouseEvent) {
    if (entry.kind === "dashboard" && (event.metaKey || event.ctrlKey)) {
      openInNewTab(entry)
      return
    }
    close()
    if (entry.kind === "page") {
      void navigate({ to: entry.to })
    } else if (entry.kind === "dashboard") {
      void navigate({
        to: "/dashboards/$dashboardId",
        params: { dashboardId: entry.id },
      })
    } else {
      void signOut()
    }
  }

  function openHighlightedInNewTab(event: ReactKeyboardEvent) {
    const entry = highlighted.current
    if (
      event.key !== "Enter" ||
      !(event.metaKey || event.ctrlKey) ||
      entry?.kind !== "dashboard"
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
            aria-label="Search dashboards and pages"
            placeholder="Search dashboards or jump to a page"
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
                    <span className="truncate">{entry.label}</span>
                    {entry.kind === "dashboard" ? (
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
          No pages or dashboards match &quot;{query.trim()}&quot;.
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
      if (!isPaletteShortcut(event)) return
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

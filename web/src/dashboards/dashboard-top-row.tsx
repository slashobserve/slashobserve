import {
  Fragment,
  useCallback,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react"
import { Link } from "@tanstack/react-router"
import { SlidersHorizontalIcon } from "lucide-react"

import { pendingPaneHeading } from "@/components/dock/dock-focus"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { cn } from "@/lib/utils"
import { TopRowInboxBell } from "@/shell/inbox-bell"

import { CloseControlsProvider } from "./controls-sheet"
import { controlsButtonId } from "./pane-openers"

type Props = {
  title: string
  folder?: { id: string; title: string } | null
  madeBy: string | null
  views?: string | null
  versionControl: ReactNode
  presence?: ReactNode
  controls: ReactNode[]
  actions: ReactNode[]
}

function Items({ nodes }: { nodes: ReactNode[] }) {
  return nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>)
}

const narrowQuery = "(max-width: 899px)"

function subscribeNarrow(onChange: () => void) {
  const media = window.matchMedia(narrowQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function useNarrow(): boolean {
  return useSyncExternalStore(
    subscribeNarrow,
    () => window.matchMedia(narrowQuery).matches,
    () => false
  )
}

function Separator() {
  return (
    <span aria-hidden="true" className="shrink-0 text-muted-foreground">
      /
    </span>
  )
}

export function DashboardTopRow({
  title,
  folder,
  madeBy,
  views,
  versionControl,
  presence,
  controls,
  actions,
}: Props) {
  const narrow = useNarrow()
  const [controlsOpen, setControlsOpen] = useState(false)
  const closeControls = useCallback(() => setControlsOpen(false), [])
  const tools = (
    <>
      <Items nodes={controls} />
      <Items nodes={actions} />
    </>
  )
  return (
    <header
      className={cn(
        "relative flex pt-3 pr-2.5 pb-2.5 pl-4",
        narrow
          ? "items-center justify-between gap-x-4"
          : "flex-wrap items-center justify-between gap-x-4 gap-y-2"
      )}
    >
      <nav
        aria-label="Breadcrumb"
        className={cn(
          "flex min-w-0 flex-nowrap items-center gap-2 text-[13px]",
          !narrow && "flex-[1_1_22rem]"
        )}
      >
        {narrow ? null : (
          <>
            <Link
              to="/dashboards"
              className="shrink-0 rounded-sm text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              Dashboards
            </Link>
            <Separator />
            {folder ? (
              <>
                <Link
                  to="/dashboards"
                  search={{ folder: folder.id }}
                  title={folder.title}
                  className="max-w-[24ch] shrink-0 truncate rounded-sm text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {folder.title}
                </Link>
                <Separator />
              </>
            ) : null}
          </>
        )}
        <h1
          aria-current="page"
          title={title}
          className="max-w-[40ch] min-w-[8ch] truncate text-[13px] font-medium"
        >
          {title}
        </h1>
        {madeBy && !narrow ? (
          <span
            title={madeBy}
            className="min-w-0 shrink-[1000] truncate text-xs text-muted-foreground"
          >
            {madeBy}
          </span>
        ) : null}
        {views ? (
          <span
            title={views}
            className="min-w-0 shrink-[1000] truncate text-xs text-muted-foreground"
          >
            {views}
          </span>
        ) : null}
        {versionControl}
        {presence}
      </nav>
      {narrow ? (
        <Sheet open={controlsOpen} onOpenChange={setControlsOpen}>
          <SheetTrigger
            id={controlsButtonId}
            render={<Button variant="outline" size="sm" />}
          >
            <SlidersHorizontalIcon aria-hidden="true" />
            Controls
          </SheetTrigger>
          <SheetContent
            side="bottom"
            className="max-h-[85svh] overflow-auto"
            finalFocus={() => pendingPaneHeading() ?? true}
          >
            <SheetHeader className="pb-0">
              <SheetTitle>Controls</SheetTitle>
              <SheetDescription>
                Time range, refresh, annotations and dashboard actions.
              </SheetDescription>
            </SheetHeader>
            <CloseControlsProvider value={closeControls}>
              <div className="flex flex-wrap items-center gap-1.5 px-4 pb-4">
                {tools}
              </div>
            </CloseControlsProvider>
          </SheetContent>
        </Sheet>
      ) : (
        <div className="flex max-w-full flex-none flex-wrap items-center gap-1.5">
          {tools}
          <TopRowInboxBell />
        </div>
      )}
    </header>
  )
}

import { Fragment, useSyncExternalStore, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { SlidersHorizontalIcon } from "lucide-react"

import { AnnotationFilter } from "@/annotations/annotation-filter"
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

type Props = {
  title: string
  folder?: { id: string; title: string } | null
  madeBy: string | null
  versionControl: ReactNode
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
  versionControl,
  controls,
  actions,
}: Props) {
  const narrow = useNarrow()
  const tools = (
    <>
      <AnnotationFilter />
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
          : "flex-col gap-y-2 @min-[900px]/dash:flex-row @min-[900px]/dash:items-center @min-[900px]/dash:justify-between @min-[900px]/dash:gap-x-4"
      )}
    >
      <nav
        aria-label="Breadcrumb"
        className="flex min-w-0 flex-nowrap items-center gap-2 text-[13px]"
      >
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
        <h1
          aria-current="page"
          title={title}
          className="max-w-[40ch] min-w-0 truncate text-[13px] font-medium"
        >
          {title}
        </h1>
        {madeBy ? (
          <span className="shrink-0 text-xs text-muted-foreground">
            {madeBy}
          </span>
        ) : null}
        {versionControl}
      </nav>
      {narrow ? (
        <Sheet>
          <SheetTrigger render={<Button variant="outline" size="sm" />}>
            <SlidersHorizontalIcon aria-hidden="true" />
            Controls
          </SheetTrigger>
          <SheetContent side="bottom" className="max-h-[85svh] overflow-auto">
            <SheetHeader className="pb-0">
              <SheetTitle>Controls</SheetTitle>
              <SheetDescription>
                Time range, refresh, annotations and dashboard actions.
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-wrap items-center gap-1.5 px-4 pb-4">
              {tools}
            </div>
          </SheetContent>
        </Sheet>
      ) : (
        <div className="flex flex-wrap items-center gap-1.5 @min-[900px]/dash:flex-none">
          {tools}
        </div>
      )}
    </header>
  )
}

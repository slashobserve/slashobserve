import { Fragment, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"

import { Mark } from "@/components/brand/mark"
import { Wordmark } from "@/components/brand/wordmark"

type Props = {
  title: string
  madeBy: string | null
  versionControl: ReactNode
  controls: ReactNode[]
  editing: ReactNode
  actions: ReactNode[]
}

function Items({ nodes }: { nodes: ReactNode[] }) {
  return nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>)
}

function Separator() {
  return (
    <span aria-hidden="true" className="text-muted-foreground">
      /
    </span>
  )
}

export function DashboardTopRow({
  title,
  madeBy,
  versionControl,
  controls,
  editing,
  actions,
}: Props) {
  return (
    <header className="relative flex flex-wrap items-center justify-between gap-x-4 gap-y-2 pt-3 pr-2.5 pb-2.5 pl-4">
      <nav
        aria-label="Breadcrumb"
        className="flex min-w-0 flex-wrap items-center gap-2 text-[13px]"
      >
        <Link
          to="/"
          aria-label="/observe home"
          className="flex items-center gap-[7px] rounded-sm no-underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Mark size={18} />
          <Wordmark size={13} tone="ink" />
        </Link>
        <Separator />
        <Link
          to="/dashboards"
          className="rounded-sm text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          Dashboards
        </Link>
        <Separator />
        <h1
          aria-current="page"
          title={title}
          className="max-w-[40ch] truncate text-[13px] font-medium"
        >
          {title}
        </h1>
        {madeBy ? (
          <span className="text-xs text-muted-foreground">{madeBy}</span>
        ) : null}
        {versionControl}
      </nav>
      <div className="flex flex-wrap items-center gap-1.5">
        {editing}
        <Items nodes={controls} />
        <Items nodes={actions} />
      </div>
    </header>
  )
}

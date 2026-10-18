import { Fragment, type ReactNode } from "react"

import { Badge } from "@/components/ui/badge"

type Props = {
  title: string
  versionBadge: string
  controls: ReactNode[]
  actions: ReactNode[]
}

function Items({ nodes }: { nodes: ReactNode[] }) {
  return nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>)
}

export function DashboardToolbar({
  title,
  versionBadge,
  controls,
  actions,
}: Props) {
  return (
    <div className="sticky top-0 z-10 border-b bg-background/95">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <h1 className="truncate text-lg font-semibold" title={title}>
            {title}
          </h1>
          <Badge variant="secondary">{versionBadge}</Badge>
        </div>
        <div className="flex flex-wrap items-center gap-2 max-sm:w-full">
          <div className="flex flex-wrap items-center gap-2 sm:contents">
            <Items nodes={controls} />
          </div>
          <div className="flex items-center gap-2 max-sm:w-full max-sm:justify-between sm:contents">
            <Items nodes={actions} />
          </div>
        </div>
      </div>
    </div>
  )
}

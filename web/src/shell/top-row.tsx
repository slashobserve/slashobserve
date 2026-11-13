import { Breadcrumb } from "./breadcrumb"
import { useCrumbs } from "./crumbs"

export function TopRow({
  setActionsHost,
  setTitleHost,
  crumbHidden,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
  setTitleHost: (element: HTMLSpanElement | null) => void
  crumbHidden: boolean
}) {
  const { crumbs } = useCrumbs()
  const hidden = crumbHidden || crumbs.length === 0
  return (
    <header
      data-crumb-hidden={hidden || undefined}
      className="@container/top flex items-center justify-between gap-4 px-6 py-[9px] data-crumb-hidden:has-[[data-slot=top-row-actions]:empty]:hidden narrow:justify-end narrow:px-4 narrow:has-[[data-slot=top-row-actions]:empty]:hidden"
    >
      {hidden ? null : (
        <div className="flex min-w-0 flex-1 items-center gap-2 narrow:hidden">
          <Breadcrumb setTitleHost={setTitleHost} />
        </div>
      )}
      <div
        ref={setActionsHost}
        data-slot="top-row-actions"
        className="ml-auto flex shrink-0 items-center gap-1.5"
      />
    </header>
  )
}

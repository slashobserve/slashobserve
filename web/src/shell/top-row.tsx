import { Breadcrumb } from "./breadcrumb"

export function TopRow({
  setActionsHost,
  setTitleHost,
  crumbHidden,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
  setTitleHost: (element: HTMLSpanElement | null) => void
  crumbHidden: boolean
}) {
  return (
    <header
      data-crumb-hidden={crumbHidden || undefined}
      className="@container/top flex items-center justify-between gap-4 py-[9px] pr-3 pl-2.5 data-crumb-hidden:has-[[data-slot=top-row-actions]:empty]:hidden narrow:justify-end narrow:px-4 narrow:has-[[data-slot=top-row-actions]:empty]:hidden"
    >
      {crumbHidden ? null : (
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

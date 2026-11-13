import { Breadcrumb } from "./breadcrumb"

export function TopRow({
  setActionsHost,
  setTitleHost,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
  setTitleHost: (element: HTMLSpanElement | null) => void
}) {
  return (
    <header className="@container/top flex items-center justify-between gap-4 py-[9px] pr-3 pl-2.5 narrow:justify-end narrow:px-4 narrow:has-[[data-slot=top-row-actions]:empty]:hidden">
      <div className="flex min-w-0 flex-1 items-center gap-2 narrow:hidden">
        <Breadcrumb setTitleHost={setTitleHost} />
      </div>
      <div
        ref={setActionsHost}
        data-slot="top-row-actions"
        className="flex shrink-0 items-center gap-1.5"
      />
    </header>
  )
}

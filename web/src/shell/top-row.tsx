import { Breadcrumb } from "./breadcrumb"

export function TopRow({
  setActionsHost,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
}) {
  return (
    <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 py-[9px] pr-3 pl-2.5 narrow:justify-end narrow:px-4 narrow:has-[[data-slot=top-row-actions]:empty]:hidden">
      <div className="flex min-w-0 items-center gap-2 narrow:hidden">
        <Breadcrumb />
      </div>
      <div
        ref={setActionsHost}
        data-slot="top-row-actions"
        className="flex items-center gap-1.5"
      />
    </header>
  )
}

import { Breadcrumb } from "./breadcrumb"
import { useCrumbs } from "./crumbs"
import { InboxBell } from "./inbox-bell"

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
    <header className="@container/top flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5 px-6 py-[9px] narrow:justify-end narrow:px-4 narrow:has-[[data-slot=top-row-actions]:empty]:hidden">
      {hidden ? null : (
        <div className="flex min-w-32 flex-1 items-center gap-2 narrow:hidden">
          <Breadcrumb setTitleHost={setTitleHost} />
        </div>
      )}
      <div
        ref={setActionsHost}
        data-slot="top-row-actions"
        className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-1.5"
      />
      <InboxBell
        place="top"
        className="grid size-8 flex-none place-items-center rounded-control text-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted narrow:hidden"
        badgeClassName="-top-0.5 -right-1 border-background"
      />
    </header>
  )
}

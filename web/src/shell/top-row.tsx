import { Breadcrumb } from "./breadcrumb"
import { PaletteButton } from "./palette-button"
import { PhoneMenu } from "./phone-menu"

export function TopRow({
  setActionsHost,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
}) {
  return (
    <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 py-[9px] pr-3 pl-2.5">
      <div className="flex min-w-0 items-center gap-2">
        <PhoneMenu />
        <Breadcrumb />
      </div>
      <div className="flex items-center gap-1.5">
        <div ref={setActionsHost} className="flex items-center gap-1.5" />
        <PaletteButton variant="top" />
      </div>
    </header>
  )
}

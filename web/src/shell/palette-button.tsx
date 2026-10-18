import { SearchIcon } from "lucide-react"

import { shortcutLabel } from "./palette-items"
import { usePaletteStore } from "./palette-store"

export function PaletteButton() {
  const setOpen = usePaletteStore((state) => state.setOpen)
  return (
    <button
      type="button"
      aria-label="Search or jump to"
      aria-keyshortcuts="Meta+K Control+K"
      onClick={() => setOpen(true)}
      className="flex h-[30px] w-[220px] max-w-[46vw] items-center gap-2 rounded-[7px] border bg-background pr-1.5 pl-[9px] text-left text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
    >
      <SearchIcon aria-hidden="true" className="size-3.5" strokeWidth={2} />
      <span className="flex-1 truncate">Search or jump to</span>
      <kbd
        aria-hidden="true"
        className="rounded-[4px] border px-[5px] font-[inherit] text-[11px]"
      >
        {shortcutLabel(navigator.userAgent)}
      </kbd>
    </button>
  )
}

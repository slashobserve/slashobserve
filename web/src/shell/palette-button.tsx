import { SearchIcon } from "lucide-react"

import { modChordLabel } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"

import { usePaletteStore } from "./palette-store"

type Props =
  | { variant: "top" }
  | { variant: "sidebar"; collapsed: boolean; onNavigate?: () => void }

const label = "Search or jump to"
const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring"

export function PaletteButton(props: Props) {
  const setOpen = usePaletteStore((state) => state.setOpen)
  const sidebar = props.variant === "sidebar" ? props : null
  const collapsed = sidebar?.collapsed ?? false
  return (
    <button
      type="button"
      aria-label={label}
      aria-keyshortcuts="Meta+K Control+K"
      title={collapsed ? label : undefined}
      onClick={() => {
        sidebar?.onNavigate?.()
        setOpen(true)
      }}
      className={cn(
        "items-center gap-2 rounded-control border bg-background text-left text-[13px] text-muted-foreground hover:text-foreground",
        focusRing,
        sidebar === null
          ? "hidden h-[30px] w-[220px] max-w-[46vw] pr-1.5 pl-[9px] narrow:flex"
          : cn("flex h-8 w-full", collapsed ? "justify-center" : "px-2.5")
      )}
    >
      <SearchIcon
        aria-hidden="true"
        className="size-3.5 flex-none"
        strokeWidth={2}
      />
      {collapsed ? null : <span className="flex-1 truncate">{label}</span>}
      {sidebar === null || collapsed ? null : (
        <kbd
          aria-hidden="true"
          className="rounded-[4px] border px-[5px] font-[inherit] text-xs"
        >
          {modChordLabel(navigator.userAgent, "K")}
        </kbd>
      )}
    </button>
  )
}

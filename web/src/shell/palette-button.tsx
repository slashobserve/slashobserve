import { SearchIcon } from "lucide-react"

import { modChordLabel } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"

import { usePaletteStore } from "./palette-store"

type Props =
  | { variant: "phone" }
  | { variant: "sidebar"; onNavigate?: () => void }
  | { variant: "rail" }

const label = "Search or jump to"
const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring"

export function PaletteButton(props: Props) {
  const setOpen = usePaletteStore((state) => state.setOpen)
  const onNavigate = props.variant === "sidebar" ? props.onNavigate : undefined
  return (
    <button
      type="button"
      aria-label={label}
      aria-keyshortcuts="Meta+K Control+K"
      onClick={() => {
        onNavigate?.()
        setOpen(true)
      }}
      className={cn(
        "text-muted-foreground hover:text-foreground",
        focusRing,
        props.variant === "phone" &&
          "grid size-11 flex-none place-items-center rounded-control text-foreground hover:bg-muted",
        props.variant === "rail" &&
          "grid size-9 place-items-center rounded-control border border-transparent hover:bg-nav-active",
        props.variant === "sidebar" &&
          "flex h-8 w-full items-center gap-2 rounded-control border bg-background px-2.5 text-left text-[13px]"
      )}
    >
      <SearchIcon
        aria-hidden="true"
        className={cn(
          "flex-none",
          props.variant === "phone" && "size-5",
          props.variant === "rail" && "size-4",
          props.variant === "sidebar" && "size-3.5"
        )}
        strokeWidth={props.variant === "rail" ? 1.8 : 2}
      />
      {props.variant === "sidebar" ? (
        <>
          <span className="flex-1 truncate">{label}</span>
          <kbd
            aria-hidden="true"
            className="rounded-[4px] border px-[5px] font-[inherit] text-xs"
          >
            {modChordLabel(navigator.userAgent, "K")}
          </kbd>
        </>
      ) : null}
    </button>
  )
}

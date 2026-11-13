import type { ReactElement, ReactNode } from "react"
import type { LucideIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

export const railItemClass =
  "relative grid size-9 place-items-center rounded-control border border-transparent text-muted-foreground no-underline outline-none hover:bg-nav-active hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:border-border aria-[current=page]:bg-nav-active aria-[current=page]:text-foreground data-[popup-open]:border-border data-[popup-open]:bg-nav-active data-[popup-open]:text-foreground"

export const railFlyoutItemClass =
  "flex h-8 items-center justify-between gap-2 rounded-control px-2 text-[13px] text-foreground no-underline outline-none data-[highlighted]:bg-muted"

export const railFlyoutLabelClass =
  "px-2 pt-2 pb-1 text-xs font-normal text-muted-foreground"

export function RailTip({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string | null
  children: ReactElement
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent
        side="right"
        sideOffset={6}
        role="tooltip"
        className="h-7 px-2 py-0 font-medium"
      >
        {label}
        {hint ? (
          <span className="font-normal text-muted-foreground">{hint}</span>
        ) : null}
      </TooltipContent>
    </Tooltip>
  )
}

export function RailFlyout({
  label,
  icon: Icon,
  active,
  children,
}: {
  label: string
  icon: LucideIcon
  active: boolean
  children: ReactNode
}) {
  return (
    <DropdownMenu modal={false}>
      <RailTip label={label}>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              aria-label={label}
              aria-current={active ? "page" : undefined}
              className={railItemClass}
            />
          }
        >
          <Icon aria-hidden="true" className="size-4" strokeWidth={1.8} />
        </DropdownMenuTrigger>
      </RailTip>
      <DropdownMenuContent
        side="right"
        align="start"
        sideOffset={6}
        alignOffset={-14}
        aria-label={label}
        className="w-[248px] rounded-card p-1"
      >
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

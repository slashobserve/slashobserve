import type { ReactNode } from "react"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

import type { TooltipSide } from "./table-layout"

export function CellTooltip({
  text,
  side,
  children,
}: {
  text: string | null
  side: TooltipSide
  children: ReactNode
}) {
  if (text === null) return <>{children}</>
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="block" />}>
        {children}
      </TooltipTrigger>
      <TooltipContent side={side} role="tooltip">
        {text}
      </TooltipContent>
    </Tooltip>
  )
}

import type { ComponentProps } from "react"

import { cn } from "@/lib/utils"

export function ScrollPane({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn("relative min-h-0 overflow-y-auto", className)}
      {...props}
    />
  )
}

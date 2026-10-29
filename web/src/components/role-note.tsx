import type { ReactNode } from "react"
import { LockIcon } from "lucide-react"

import { cn } from "@/lib/utils"

export function RoleNote({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <p
      className={cn(
        "flex items-center gap-2 text-[13px] text-muted-foreground",
        className
      )}
    >
      <LockIcon aria-hidden="true" className="size-3.5 flex-none" />
      <span>{children}</span>
    </p>
  )
}

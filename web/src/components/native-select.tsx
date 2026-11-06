import type { ComponentProps } from "react"
import { ChevronDownIcon } from "lucide-react"

import { cn } from "@/lib/utils"

type NativeSelectProps = Omit<ComponentProps<"select">, "size"> & {
  size?: "default" | "sm"
}

const sizeClass = {
  default: "h-8 pr-8 pl-2.5 text-[13px]",
  sm: "h-7 pr-7 pl-2 text-xs",
}

export function NativeSelect({
  size = "default",
  className,
  ...props
}: NativeSelectProps) {
  return (
    <span className="relative inline-flex min-w-0">
      <select
        className={cn(
          "min-w-0 appearance-none rounded-control border border-input bg-background text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
          sizeClass[size],
          className
        )}
        {...props}
      />
      <ChevronDownIcon
        aria-hidden
        className="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground"
      />
    </span>
  )
}

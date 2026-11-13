import { cn } from "@/lib/utils"

export function CountBadge({
  count,
  className,
}: {
  count: string
  className?: string
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "absolute grid h-4 min-w-4 place-items-center rounded-full border-2 bg-primary px-1 text-[10px] leading-none font-semibold text-primary-foreground",
        className
      )}
    >
      {count}
    </span>
  )
}

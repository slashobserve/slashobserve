import { slashPath } from "@/components/brand/mark"
import { cn } from "@/lib/utils"

type Props = {
  size?: number
  variant?: "slash" | "tile"
  className?: string
}

export function AgentMark({ size = 12, variant = "slash", className }: Props) {
  if (variant === "tile") {
    return (
      <span
        aria-hidden="true"
        className={cn(
          "grid size-[26px] flex-none place-items-center rounded-[7px] bg-primary text-primary-foreground",
          className
        )}
      >
        <svg width={14} height={14} viewBox="0 0 24 24" fill="none">
          <path
            d={slashPath}
            stroke="currentColor"
            strokeWidth="2.4"
            strokeLinecap="round"
          />
        </svg>
      </span>
    )
  }
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
      className={cn("block flex-none", className)}
    >
      <path
        d={slashPath}
        stroke="currentColor"
        strokeWidth="2.6"
        strokeLinecap="round"
      />
    </svg>
  )
}

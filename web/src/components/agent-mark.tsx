import { slashPath } from "@/components/brand/mark"
import { cn } from "@/lib/utils"

type Props = { size?: number; className?: string }

export function AgentMark({ size = 12, className }: Props) {
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

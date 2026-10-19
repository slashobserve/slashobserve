import { cn } from "@/lib/utils"

export const slashPath = "M14.6 5.6 9.4 18.4"

type Props = {
  size?: number
  variant?: "ink" | "brand" | "rail"
  className?: string
}

export function Mark({ size = 22, variant = "ink", className }: Props) {
  const px = Math.max(16, size)
  const rail = variant === "rail"
  return (
    <svg
      width={px}
      height={px}
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={cn("block flex-none", className)}
    >
      <rect
        x={rail ? 2 : 0}
        y={rail ? 2 : 0}
        width={rail ? 20 : 24}
        height={rail ? 20 : 24}
        rx={rail ? 5 : 5.8}
        className={variant === "brand" ? "fill-brand" : "fill-foreground"}
      />
      <path
        d={rail ? "M14.5 6.5 9.5 17.5" : slashPath}
        strokeWidth={rail ? 2.4 : 2.6}
        strokeLinecap="round"
        fill="none"
        className={
          variant === "brand"
            ? "stroke-white"
            : variant === "rail"
              ? "stroke-chrome"
              : "stroke-background"
        }
      />
    </svg>
  )
}

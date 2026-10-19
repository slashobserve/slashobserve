import { cn } from "@/lib/utils"

export const slashPath = "M14.6 5.6 9.4 18.4"

type Props = { size?: number; variant?: "ink" | "brand"; className?: string }

export function Mark({ size = 22, variant = "ink", className }: Props) {
  const px = Math.max(16, size)
  return (
    <svg
      width={px}
      height={px}
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={cn("block flex-none", className)}
    >
      <rect
        width="24"
        height="24"
        rx="5.8"
        className={variant === "brand" ? "fill-brand" : "fill-foreground"}
      />
      <path
        d={slashPath}
        strokeWidth="2.6"
        strokeLinecap="round"
        fill="none"
        className={variant === "brand" ? "stroke-white" : "stroke-background"}
      />
    </svg>
  )
}

import { cn } from "@/lib/utils"

type Tone = "brand" | "ink" | "on-brand"

type Props = { size?: number; tone?: Tone; className?: string }

export function Wordmark({ size = 14, tone = "brand", className }: Props) {
  const px = Math.max(12, size)
  return (
    <span
      className={cn(
        "inline-block font-sans leading-none font-semibold tracking-[-0.035em] whitespace-nowrap",
        tone === "on-brand" ? "text-white" : "text-foreground",
        className
      )}
      style={{ fontSize: px }}
    >
      <span
        className={cn(
          "mr-[0.02em]",
          tone === "brand"
            ? "text-brand"
            : tone === "on-brand"
              ? "text-white"
              : "text-foreground"
        )}
      >
        /
      </span>
      observe
    </span>
  )
}

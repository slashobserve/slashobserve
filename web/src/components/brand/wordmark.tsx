import { cn } from "@/lib/utils"

type Props = { size?: number; className?: string }

export function Wordmark({ size = 14, className }: Props) {
  const px = Math.max(12, size)
  return (
    <span
      translate="no"
      className={cn(
        "inline-block font-sans leading-none font-semibold tracking-[-0.01em] whitespace-nowrap text-foreground",
        className
      )}
      style={{ fontSize: px }}
    >
      <span className="mr-[0.06em]">/</span>observe
    </span>
  )
}

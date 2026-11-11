import type { AuthorMarkModel, PersonTint } from "@/lib/author"
import { cn } from "@/lib/utils"

const personClasses: Record<PersonTint, string> = {
  1: "bg-person-1 text-person-1-foreground",
  2: "bg-person-2 text-person-2-foreground",
  3: "bg-person-3 text-person-3-foreground",
  4: "bg-person-4 text-person-4-foreground",
  5: "bg-person-5 text-person-5-foreground",
  6: "bg-person-6 text-person-6-foreground",
}

const sizeClasses = { 20: "size-5", 24: "size-6" } as const

function kindClasses(mark: AuthorMarkModel): string {
  switch (mark.kind) {
    case "observe-agent":
      return "bg-primary text-primary-foreground text-[12px] font-bold"
    case "outside-agent":
      return "border-[1.5px] border-foreground bg-card text-foreground text-[9px] font-semibold"
    case "person":
      return cn("text-[10px] font-semibold", personClasses[mark.tint ?? 1])
  }
}

type AuthorMarkProps = {
  mark: AuthorMarkModel
  size?: 20 | 24
  className?: string
}

export function AuthorMark({ mark, size = 24, className }: AuthorMarkProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "grid flex-none place-items-center rounded-full leading-none",
        sizeClasses[size],
        kindClasses(mark),
        className
      )}
    >
      {mark.text}
    </span>
  )
}

type AuthorStackProps = {
  marks: readonly { key: string; mark: AuthorMarkModel; label: string }[]
  label: string
  max?: number
}

export function AuthorStack({ marks, label, max = 5 }: AuthorStackProps) {
  const shown = marks.slice(0, max)
  const hidden = marks.length - shown.length
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="inline-flex items-center gap-[3px]"
    >
      {shown.map((entry) => (
        <AuthorMark key={entry.key} mark={entry.mark} />
      ))}
      {hidden > 0 ? (
        <span className="text-[12px] text-muted-foreground">+{hidden}</span>
      ) : null}
    </span>
  )
}

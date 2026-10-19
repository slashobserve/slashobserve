import { MessageSquareIcon } from "lucide-react"

type Props = {
  pressed: boolean
  openCount: number
  disabled?: boolean
  onToggle: () => void
}

export function ActivityToggle({
  pressed,
  openCount,
  disabled,
  onToggle,
}: Props) {
  const label = `${pressed ? "Hide" : "Show"} activity${
    openCount > 0
      ? `, ${openCount} open ${openCount === 1 ? "comment" : "comments"}`
      : ""
  }`
  return (
    <button
      type="button"
      aria-pressed={pressed}
      aria-label={label}
      data-comments-trigger=""
      disabled={disabled}
      onClick={onToggle}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <MessageSquareIcon aria-hidden="true" className="size-4" />
      {openCount > 0 ? <span className="tabular-nums">{openCount}</span> : null}
    </button>
  )
}

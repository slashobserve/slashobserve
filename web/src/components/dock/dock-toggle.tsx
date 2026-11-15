import type { Ref } from "react"
import { MessageSquareIcon } from "lucide-react"

type Props = {
  label: string
  count: number
  pressed: boolean
  disabled?: boolean
  onToggle: () => void
  ref?: Ref<HTMLButtonElement>
}

export function DockToggle({
  label,
  count,
  pressed,
  disabled,
  onToggle,
  ref,
}: Props) {
  return (
    <button
      ref={ref}
      type="button"
      aria-pressed={pressed}
      aria-label={label}
      disabled={disabled}
      onClick={onToggle}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <MessageSquareIcon aria-hidden="true" className="size-4" />
      {count > 0 ? <span className="tabular-nums">{count}</span> : null}
    </button>
  )
}

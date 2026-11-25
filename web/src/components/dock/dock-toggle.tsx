import type { Ref } from "react"
import { MessageSquareIcon } from "lucide-react"

import { closeInboxIfOpen, useInboxPanelOpen } from "@/inbox/inbox-panel-store"

type Props = {
  id?: string
  label: string
  count: number
  pressed: boolean
  disabled?: boolean
  onToggle: () => void
  ref?: Ref<HTMLButtonElement>
}

export function DockToggle({
  id,
  label,
  count,
  pressed,
  disabled,
  onToggle,
  ref,
}: Props) {
  const inboxOpen = useInboxPanelOpen()
  const yielded = pressed && inboxOpen
  return (
    <button
      ref={ref}
      id={id}
      type="button"
      aria-pressed={pressed && !yielded}
      aria-label={label}
      disabled={disabled}
      onClick={() => {
        if (yielded && closeInboxIfOpen()) return
        onToggle()
      }}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <MessageSquareIcon aria-hidden="true" className="size-4" />
      {count > 0 ? <span className="tabular-nums">{count}</span> : null}
    </button>
  )
}

import { useRef, type KeyboardEvent } from "react"

import { cn } from "@/lib/utils"
import type { EditorView } from "@/panel-editor/panel-editor"

const views: { view: EditorView; label: string }[] = [
  { view: "editor", label: "Editor" },
  { view: "json", label: "JSON" },
]

type Props = {
  view: EditorView
  onChange: (view: EditorView) => void
}

export function EditorViewSwitch({ view, onChange }: Props) {
  const buttons = useRef<Partial<Record<EditorView, HTMLButtonElement>>>({})

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const step =
      event.key === "ArrowRight" || event.key === "ArrowDown"
        ? 1
        : event.key === "ArrowLeft" || event.key === "ArrowUp"
          ? -1
          : 0
    if (step === 0) return
    event.preventDefault()
    const at = views.findIndex((item) => item.view === view)
    const next = views[(at + step + views.length) % views.length].view
    onChange(next)
    buttons.current[next]?.focus()
  }

  return (
    <div
      role="radiogroup"
      aria-label="Editor view"
      onKeyDown={onKeyDown}
      className="flex overflow-hidden rounded-control border"
    >
      {views.map((item, index) => {
        const checked = item.view === view
        return (
          <button
            key={item.view}
            ref={(node) => {
              if (node) buttons.current[item.view] = node
              else delete buttons.current[item.view]
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => onChange(item.view)}
            className={cn(
              "h-[26px] px-2.5 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
              index > 0 && "border-l",
              checked && "bg-muted"
            )}
          >
            {item.label}
          </button>
        )
      })}
    </div>
  )
}

import { useId, useRef, type KeyboardEvent, type ReactNode } from "react"
import { CodeIcon, ListFilterIcon, SparklesIcon } from "lucide-react"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import type { InputMode } from "./input-mode"

const shortcutText = "Switch input mode (Ctrl+I or Cmd+I)"
const noModelText = "Add a model key to ask in plain language"
const moveKeys = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"])

const icons: Record<InputMode, ReactNode> = {
  ask: <SparklesIcon aria-hidden="true" />,
  builder: <ListFilterIcon aria-hidden="true" />,
  code: <CodeIcon aria-hidden="true" />,
}

export function InputModeToggle({
  mode,
  modes,
  labels,
  agentAvailable,
  onChange,
}: {
  mode: InputMode
  modes: readonly InputMode[]
  labels: Record<InputMode, string>
  agentAvailable: boolean
  onChange: (mode: InputMode) => void
}) {
  const noModelId = useId()
  const buttons = useRef<Record<InputMode, HTMLButtonElement | null>>({
    ask: null,
    builder: null,
    code: null,
  })

  function choose(next: InputMode) {
    if (next === "ask" && !agentAvailable) return
    if (next !== mode) onChange(next)
    buttons.current[next]?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>, from: InputMode) {
    if (!moveKeys.has(event.key)) return
    event.preventDefault()
    const step = event.key === "ArrowLeft" || event.key === "ArrowUp" ? -1 : 1
    const index = modes.indexOf(from)
    choose(modes[(index + step + modes.length) % modes.length])
  }

  function option(value: InputMode) {
    const selected = mode === value
    const unavailable = value === "ask" && !agentAvailable
    return (
      <Tooltip key={value}>
        <TooltipTrigger
          render={
            <button
              ref={(element) => {
                buttons.current[value] = element
              }}
              type="button"
              role="radio"
              aria-checked={selected}
              aria-disabled={unavailable || undefined}
              aria-describedby={unavailable ? noModelId : undefined}
              tabIndex={selected ? 0 : -1}
              onClick={() => choose(value)}
              onKeyDown={(event) => onKeyDown(event, value)}
              className={cn(
                "inline-flex h-full items-center gap-1.5 rounded-chip border px-2.5 text-[13px] font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-3.5",
                selected
                  ? "border-border bg-background text-foreground"
                  : "border-transparent text-muted-foreground",
                unavailable && "cursor-not-allowed opacity-60"
              )}
            />
          }
        >
          {icons[value]}
          {labels[value]}
        </TooltipTrigger>
        <TooltipContent>
          {unavailable ? noModelText : shortcutText}
        </TooltipContent>
      </Tooltip>
    )
  }

  return (
    <div
      role="radiogroup"
      aria-label="Input mode"
      aria-keyshortcuts="Control+I Meta+I"
      className="inline-flex h-8 items-center gap-0.5 rounded-control bg-muted p-0.5 narrow:h-11"
    >
      {modes.map(option)}
      {agentAvailable ? null : (
        <span id={noModelId} className="sr-only">
          {noModelText}
        </span>
      )}
    </div>
  )
}

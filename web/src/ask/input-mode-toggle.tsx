import { useId, useRef, type KeyboardEvent, type ReactNode } from "react"
import { CodeIcon, SparklesIcon } from "lucide-react"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import type { InputMode } from "./input-mode"

const shortcutText = "Switch between Ask and Query (Ctrl+I or Cmd+I)"
const noModelText = "Add a model key to ask in plain language"
const moveKeys = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"])

export function InputModeToggle({
  mode,
  agentAvailable,
  onChange,
}: {
  mode: InputMode
  agentAvailable: boolean
  onChange: (mode: InputMode) => void
}) {
  const noModelId = useId()
  const buttons = useRef<Record<InputMode, HTMLButtonElement | null>>({
    ask: null,
    query: null,
  })

  function choose(next: InputMode) {
    if (next === "ask" && !agentAvailable) return
    if (next !== mode) onChange(next)
    buttons.current[next]?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>, from: InputMode) {
    if (!moveKeys.has(event.key)) return
    event.preventDefault()
    choose(from === "ask" ? "query" : "ask")
  }

  function option(value: InputMode, icon: ReactNode, word: string) {
    const selected = mode === value
    const unavailable = value === "ask" && !agentAvailable
    return (
      <Tooltip>
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
                "inline-flex h-full items-center gap-1 rounded-control border px-1.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-3.5",
                selected
                  ? "border-border bg-background text-foreground"
                  : "border-transparent text-muted-foreground",
                unavailable && "cursor-not-allowed opacity-60"
              )}
            />
          }
        >
          {icon}
          {word}
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
      className="inline-flex h-6 items-center gap-0.5 rounded-control bg-muted p-0.5"
    >
      {option("ask", <SparklesIcon aria-hidden="true" />, "Ask")}
      {option("query", <CodeIcon aria-hidden="true" />, "Query")}
      {agentAvailable ? null : (
        <span id={noModelId} className="sr-only">
          {noModelText}
        </span>
      )}
    </div>
  )
}

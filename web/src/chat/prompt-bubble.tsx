import { useId, useState } from "react"

import { CopyButton } from "@/components/copy-button"
import { cn } from "@/lib/utils"

import { isLongPrompt } from "./prompt-length"

export function PromptBubble({ prompt }: { prompt: string }) {
  const bodyId = useId()
  const [expanded, setExpanded] = useState(false)
  const long = isLongPrompt(prompt)
  return (
    <div className="group/bubble flex max-w-[80%] flex-col items-end gap-1 self-end">
      <div
        id={bodyId}
        className={cn(
          "w-full rounded-xl border px-3.5 py-2.5 text-sm wrap-anywhere whitespace-pre-wrap",
          long && !expanded && "line-clamp-6"
        )}
      >
        {prompt}
      </div>
      <div className="flex items-center gap-2">
        {long ? (
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls={bodyId}
            className="rounded-sm text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? "Show less" : "Show the whole message"}
          </button>
        ) : null}
        <CopyButton
          text={prompt}
          label="Copy message"
          variant="ghost"
          size="icon-xs"
          className="opacity-0 group-focus-within/bubble:opacity-100 group-hover/bubble:opacity-100 [@media(hover:none)]:opacity-100"
        />
      </div>
    </div>
  )
}

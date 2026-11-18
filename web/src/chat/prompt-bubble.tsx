import { useId, useState } from "react"

import { cn } from "@/lib/utils"

import { isLongPrompt } from "./prompt-length"

export function PromptBubble({ prompt }: { prompt: string }) {
  const bodyId = useId()
  const [expanded, setExpanded] = useState(false)
  const long = isLongPrompt(prompt)
  return (
    <div className="flex max-w-[80%] flex-col items-end gap-1 self-end">
      <div
        id={bodyId}
        className={cn(
          "w-full rounded-xl border px-3.5 py-2.5 text-sm wrap-anywhere whitespace-pre-wrap",
          long && !expanded && "line-clamp-6"
        )}
      >
        {prompt}
      </div>
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
    </div>
  )
}

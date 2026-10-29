import { useEffect } from "react"
import { useQuery } from "@tanstack/react-query"
import { LockIcon, UsersIcon } from "lucide-react"

import { useCoarsePointer } from "@/lib/coarse-pointer"

import { promptsQuery } from "./queries"
import { scopeOf } from "./scope"
import type { usePromptPicker } from "./use-prompt-picker"

type Props = {
  picker: ReturnType<typeof usePromptPicker>
  principalId: string
}

function emptyText(
  status: Props["picker"]["status"],
  hasPrompts: boolean
): string {
  if (status === "loading") return "Loading saved prompts..."
  if (status === "error") return "Could not load saved prompts."
  return hasPrompts ? "No saved prompts match." : "No saved prompts yet."
}

export function PromptPicker({ picker, principalId }: Props) {
  const coarse = useCoarsePointer()
  const { activeIndex, optionId, matches, status, total } = picker
  const showMessage = status !== "ready" || matches.length === 0
  const { data: promptCount } = useQuery({
    ...promptsQuery(),
    select: (data) => data.prompts.length,
  })
  const message = emptyText(status, (promptCount ?? 0) > 0)

  useEffect(() => {
    if (activeIndex < 0) return
    document
      .getElementById(optionId(activeIndex))
      ?.scrollIntoView({ block: "nearest" })
  }, [activeIndex, optionId])

  return (
    <div className="absolute inset-x-0 top-[calc(100%+6px)] z-20 flex flex-col overflow-hidden rounded-card border bg-background">
      <div
        aria-hidden="true"
        className="flex justify-between px-3 pt-[9px] pb-1.5 text-xs text-muted-foreground"
      >
        <span>Saved prompts</span>
        {coarse ? null : <span>Enter fills the box, then edit or send</span>}
      </div>
      <ul
        id={picker.listboxId}
        role="listbox"
        aria-label="Saved prompts"
        className="max-h-[296px] overflow-y-auto"
      >
        {matches.map((prompt, index) => {
          const scope = scopeOf(prompt, principalId)
          const ScopeIcon = scope.shared ? UsersIcon : LockIcon
          return (
            <li
              key={prompt.id}
              id={optionId(index)}
              role="option"
              aria-selected={index === activeIndex}
              className="flex cursor-pointer items-center gap-3 px-3 py-[9px] aria-selected:bg-muted"
              onMouseDown={(event) => event.preventDefault()}
              onMouseMove={() => picker.setActive(index)}
              onClick={() => picker.fill(index)}
            >
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-[13px] font-medium">
                  {prompt.name}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {prompt.body}
                </span>
              </span>
              <span className="flex flex-none items-center gap-[5px] text-xs text-muted-foreground">
                <ScopeIcon aria-hidden="true" className="size-3" />
                {scope.text}
              </span>
            </li>
          )
        })}
      </ul>
      {showMessage ? (
        <p className="px-3 py-2.5 text-[13px] text-muted-foreground">
          {message}
        </p>
      ) : null}
      <div className="border-t px-3 py-2 text-xs text-muted-foreground">
        {total > matches.length
          ? `Showing ${matches.length} of ${total}. `
          : ""}
        Type to filter. Save any chat as a prompt from History.
      </div>
    </div>
  )
}

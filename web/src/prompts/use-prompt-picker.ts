import { useCallback, useId, useState, type KeyboardEvent } from "react"
import { useQuery } from "@tanstack/react-query"

import { matchPrompts, pickerLimit, pickerQuery, stepIndex } from "./match"
import { promptsQuery, type SavedPrompt } from "./queries"

type PickerStatus = "loading" | "error" | "ready"

type TextareaAttributes = {
  "aria-autocomplete"?: "list"
  "aria-controls"?: string
  "aria-activedescendant"?: string
}

function openAnnouncement(status: PickerStatus, total: number): string {
  if (status === "loading") return "Loading saved prompts."
  if (status === "error") return "Could not load saved prompts."
  if (total === 0) return "No saved prompts match."
  const count = total === 1 ? "1 saved prompt" : `${total} saved prompts`
  return `${count}. Up and down arrows to choose, Enter to fill.`
}

export function usePromptPicker({
  enabled,
  value,
  onFill,
}: {
  enabled: boolean
  value: string
  onFill: (body: string) => void
}) {
  const query = useQuery({ ...promptsQuery(), enabled })
  const listboxId = useId()
  const [dismissed, setDismissed] = useState(false)
  const [activeState, setActiveState] = useState({ query: "", index: 0 })
  const [filledName, setFilledName] = useState<string | null>(null)

  const text = pickerQuery(value)
  const queryText = text ?? ""
  const open = enabled && text !== null && !dismissed
  const status: PickerStatus =
    query.data !== undefined ? "ready" : query.isError ? "error" : "loading"
  const found = matchPrompts(query.data ?? [], queryText)
  const matches: SavedPrompt[] = found.slice(0, pickerLimit)
  const total = found.length
  const storedIndex = activeState.query === queryText ? activeState.index : 0
  const activeIndex =
    matches.length === 0 ? -1 : Math.min(storedIndex, matches.length - 1)

  const optionId = useCallback(
    (index: number) => `${listboxId}-option-${index}`,
    [listboxId]
  )

  function setActive(index: number) {
    setActiveState({ query: queryText, index })
  }

  function dismiss() {
    if (text !== null) setDismissed(true)
  }

  function fill(index: number) {
    const prompt = matches[index]
    if (prompt === undefined) return
    onFill(prompt.body)
    setDismissed(pickerQuery(prompt.body) !== null)
    setFilledName(prompt.name)
  }

  function onValueChange(next: string) {
    setFilledName(null)
    if (pickerQuery(next) === null) setDismissed(false)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
    if (!enabled || text === null) return false
    if (!open) {
      if (event.key !== "ArrowDown") return false
      event.preventDefault()
      setDismissed(false)
      return true
    }
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp":
        event.preventDefault()
        setActive(
          stepIndex(
            Math.max(activeIndex, 0),
            matches.length,
            event.key === "ArrowDown" ? 1 : -1
          )
        )
        return true
      case "Enter":
        if (event.shiftKey || event.nativeEvent.isComposing) return false
        event.preventDefault()
        if (activeIndex >= 0) fill(activeIndex)
        return true
      case "Escape":
        event.preventDefault()
        event.stopPropagation()
        dismiss()
        return true
      default:
        return false
    }
  }

  const textareaAttributes: TextareaAttributes = enabled
    ? {
        "aria-autocomplete": "list",
        ...(open ? { "aria-controls": listboxId } : {}),
        ...(open && activeIndex >= 0
          ? { "aria-activedescendant": optionId(activeIndex) }
          : {}),
      }
    : {}

  const announcement = open
    ? openAnnouncement(status, total)
    : filledName !== null
      ? `Filled "${filledName}". Edit it or press Enter to send.`
      : ""

  return {
    open,
    status,
    matches,
    total,
    activeIndex,
    listboxId,
    optionId,
    announcement,
    textareaAttributes,
    setActive,
    fill,
    onValueChange,
    dismiss,
    handleKeyDown,
  }
}

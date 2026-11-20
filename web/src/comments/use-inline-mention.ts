import { useState } from "react"

import { mentionTriggerAt } from "./mentions"

export type InlineMention = {
  open: boolean
  replaceFrom: number | null
  onOpenChange: (open: boolean) => void
  typed: (previous: string, value: string, caret: number) => void
}

export function useInlineMention(): InlineMention {
  const [open, setOpen] = useState(false)
  const [replaceFrom, setReplaceFrom] = useState<number | null>(null)

  function onOpenChange(next: boolean) {
    setOpen(next)
    if (!next) setReplaceFrom(null)
  }

  function typed(previous: string, value: string, caret: number) {
    if (value.length <= previous.length) return
    const trigger = mentionTriggerAt(value, caret)
    if (trigger === null) return
    setReplaceFrom(trigger)
    setOpen(true)
  }

  return { open, replaceFrom, onOpenChange, typed }
}

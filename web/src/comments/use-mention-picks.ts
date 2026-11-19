import { useCallback, useState } from "react"

import {
  addPick,
  toDisplay,
  toWire,
  type MentionOption,
  type MentionPick,
} from "./mentions"

export type MentionPicks = {
  wire: (text: string) => string
  pick: (option: MentionOption) => string
  adopt: (wire: string) => string
}

export function useMentionPicks(
  initial: readonly MentionPick[] = []
): MentionPicks {
  const [picks, setPicks] = useState<MentionPick[]>([...initial])
  const wire = useCallback((text: string) => toWire(text, picks), [picks])
  const pick = useCallback(
    (option: MentionOption) => {
      if (option.value === "agent:observe") return option.token
      const added = addPick(picks, option.label, option.token)
      setPicks(added.picks)
      return added.display
    },
    [picks]
  )
  const adopt = useCallback((body: string) => {
    const shown = toDisplay(body)
    setPicks(shown.picks)
    return shown.text
  }, [])
  return { wire, pick, adopt }
}

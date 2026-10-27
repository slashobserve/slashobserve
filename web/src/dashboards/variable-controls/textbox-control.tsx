import { useId, useState } from "react"

import type { TextboxVariable } from "@/spec"

import type { VariableState } from "../use-variables"
import { VariableBox, variableLabel } from "./variable-box"

type Props = {
  variable: TextboxVariable
  state: VariableState
  onChange: (values: string[]) => void
}

export function TextboxControl({ variable, state, onChange }: Props) {
  const id = useId()
  const selected = state.selected[0] ?? ""
  const [draft, setDraft] = useState(selected)
  const [seen, setSeen] = useState(selected)
  const [submitted, setSubmitted] = useState<string | null>(null)
  if (seen !== selected) {
    setSeen(selected)
    setDraft(selected)
    setSubmitted(null)
  }

  function commit() {
    if (draft === selected || draft === submitted) return
    setSubmitted(draft)
    onChange([draft])
  }

  return (
    <VariableBox variable={variable} id={id}>
      <input
        id={id}
        type="text"
        aria-label={variableLabel(variable)}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.nativeEvent.isComposing) commit()
        }}
        className="w-40 min-w-0 bg-transparent px-[9px] text-[13px] outline-none"
      />
    </VariableBox>
  )
}

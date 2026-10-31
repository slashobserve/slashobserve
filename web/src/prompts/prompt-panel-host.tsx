import { useEffect } from "react"
import { useRouteContext } from "@tanstack/react-router"

import { canSavePrompts } from "./access"
import { PromptFormPanel } from "./prompt-form-panel"
import { usePromptPanel } from "./prompt-panel-store"

export function PromptPanelHost() {
  const { principal } = useRouteContext({ from: "/_authed" })
  const target = usePromptPanel((state) => state.target)
  const closePanel = usePromptPanel((state) => state.closePanel)

  useEffect(() => () => usePromptPanel.getState().closePanel(), [])

  if (target === null || !canSavePrompts(principal)) return null
  return (
    <PromptFormPanel
      key={JSON.stringify(target)}
      target={target}
      onClose={closePanel}
    />
  )
}

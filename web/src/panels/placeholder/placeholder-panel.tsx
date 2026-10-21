import { PuzzleIcon } from "lucide-react"

import type { StaticPanelRenderer } from "../types"

const PlaceholderPanel: StaticPanelRenderer = ({ panel }) => {
  if (panel.type !== "placeholder") return null
  return (
    <div
      data-placeholder
      className="flex size-full flex-col items-center justify-center gap-1 p-3 text-center"
    >
      <PuzzleIcon aria-hidden="true" className="size-5 text-muted-foreground" />
      <p className="text-sm">{panel.reason}</p>
      {panel.sourceType ? (
        <p className="text-xs text-muted-foreground">
          Grafana panel type: {panel.sourceType}
        </p>
      ) : null}
    </div>
  )
}

export default PlaceholderPanel

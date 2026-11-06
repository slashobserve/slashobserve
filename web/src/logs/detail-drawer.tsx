import type { RefObject } from "react"
import { useQuery } from "@tanstack/react-query"

import { labelNamesQuery } from "@/panel-editor/queries"

import { ContextPanel } from "./context-panel"
import type { ContextRequest } from "./logs-results"
import { useLogsWorkspace } from "./workspace"

export function DetailDrawer({
  selection,
  onClose,
  trigger,
}: {
  selection: ContextRequest | null
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}) {
  const { datasource } = useLogsWorkspace()
  const indexLabels = useQuery({
    ...labelNamesQuery(datasource.uid, {}),
    enabled: datasource.type === "loki",
  })
  if (selection === null) return null
  return (
    <ContextPanel
      datasource={datasource}
      anchor={selection}
      indexedLabels={indexLabels.data?.labels}
      onClose={onClose}
      trigger={trigger}
    />
  )
}

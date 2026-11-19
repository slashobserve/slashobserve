import { useDatasources } from "@/datasources/queries"
import { newPanelDraft, useNewPanels } from "@/panel-editor/new-panel"
import type { Dashboard } from "@/spec"

import { editPanelQuery } from "./panel-actions"
import { useSetDashboardSearch } from "./search"

export function useAddPanel(
  dashboardId: string,
  spec: Dashboard
): (() => void) | null {
  const datasources = useDatasources()
  const setSearch = useSetDashboardSearch()
  const open = useNewPanels((state) => state.open)
  const draft = newPanelDraft(spec, datasources.data ?? [])
  if (draft === null) return null
  return () => {
    open(dashboardId, draft)
    editPanelQuery(setSearch, draft.id)
  }
}

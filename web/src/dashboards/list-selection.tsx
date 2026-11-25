import { createContext, useContext, type ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"

import type { DashboardSummary } from "./dashboard-columns"

export type ListSelection = {
  selectable: readonly string[]
  selected: ReadonlySet<string>
  toggle: (id: string) => void
  togglePage: () => void
}

const ListSelectionContext = createContext<ListSelection | null>(null)

export function ListSelectionProvider({
  value,
  children,
}: {
  value: ListSelection | null
  children: ReactNode
}) {
  return (
    <ListSelectionContext.Provider value={value}>
      {children}
    </ListSelectionContext.Provider>
  )
}

const gutterPlacement =
  "absolute top-3 left-0 narrow:static narrow:mr-2.5 narrow:inline-flex narrow:align-middle"

export function RowSelectBox({ dashboard }: { dashboard: DashboardSummary }) {
  const selection = useContext(ListSelectionContext)
  if (selection === null || !selection.selectable.includes(dashboard.id)) {
    return null
  }
  return (
    <Checkbox
      aria-label={`Select ${dashboard.title}`}
      checked={selection.selected.has(dashboard.id)}
      onCheckedChange={() => selection.toggle(dashboard.id)}
      className={gutterPlacement}
    />
  )
}

export function TitleHeader() {
  const selection = useContext(ListSelectionContext)
  if (selection === null || selection.selectable.length === 0) return "Title"
  const chosen = selection.selectable.filter((id) =>
    selection.selected.has(id)
  ).length
  return (
    <span className="inline-flex items-center gap-2.5">
      <Checkbox
        aria-label="Select every dashboard on this page"
        checked={chosen === selection.selectable.length}
        indeterminate={chosen > 0 && chosen < selection.selectable.length}
        onCheckedChange={selection.togglePage}
      />
      Title
    </span>
  )
}

import { ActionButton } from "../../actions/action-button"
import type { FieldAction } from "../../actions/field-actions"
import type { Cell } from "../table-model"

export function ActionsCell({
  cell,
  actions,
  onAction,
}: {
  cell: Cell
  actions: FieldAction[]
  onAction: (action: FieldAction) => void
}) {
  return (
    <>
      <span className="sr-only">{cell.text}</span>
      <span className="inline-flex flex-wrap items-center gap-1">
        {actions.map((action) => (
          <ActionButton
            key={`${action.override ?? "panel"}:${action.index}`}
            action={action}
            size="xs"
            onRun={() => onAction(action)}
          />
        ))}
      </span>
    </>
  )
}

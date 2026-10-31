import { DropdownMenuItem } from "@/components/ui/dropdown-menu"

import type { FieldAction } from "./field-actions"

export function ActionMenuItems({
  actions,
  reason,
  onAction,
}: {
  actions: FieldAction[]
  reason: string | null
  onAction: (action: FieldAction) => void
}) {
  return actions.map((action) => (
    <DropdownMenuItem
      key={`${action.override ?? "panel"}:${action.index}`}
      disabled={reason !== null}
      title={reason ?? undefined}
      onClick={() => onAction(action)}
      className="gap-2"
    >
      {action.color === null ? null : (
        <span
          aria-hidden="true"
          className="size-2 shrink-0 rounded-full"
          style={{ backgroundColor: action.color }}
        />
      )}
      {action.title}
    </DropdownMenuItem>
  ))
}

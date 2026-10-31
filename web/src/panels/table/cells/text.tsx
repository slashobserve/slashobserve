import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import type { FieldAction } from "../../actions/field-actions"
import type { ResolvedLink } from "../../field-config"
import type { Cell } from "../table-model"
import { MoreLinks } from "./more-links"
import { ValueContent } from "./value-content"

export function TextCell({
  cell,
  links,
  actions,
  onAction,
  reason,
  tinted,
}: {
  cell: Cell
  links: ResolvedLink[]
  actions: FieldAction[]
  onAction: (action: FieldAction) => void
  reason: string | null
  tinted: boolean
}) {
  const [first, ...more] = links
  const style = tinted && cell.color ? { color: cell.color } : undefined
  const content = <ValueContent cell={cell} />
  const menu =
    more.length > 0 || actions.length > 0 ? (
      <MoreLinks
        links={more}
        actions={actions}
        onAction={onAction}
        reason={reason}
      />
    ) : null
  if (first) {
    return (
      <span className="inline-flex items-center gap-1">
        <a
          {...anchorProps(first)}
          title={
            [first.title, cell.word].filter(Boolean).join(" - ") || undefined
          }
          style={style}
          className="inline-flex items-center gap-1 underline decoration-border underline-offset-4 hover:decoration-current"
        >
          {content}
          {first.newTab ? <NewTabMark /> : null}
        </a>
        {menu}
      </span>
    )
  }
  if (style || menu) {
    return (
      <span className="inline-flex items-center gap-1" style={style}>
        {content}
        {menu}
      </span>
    )
  }
  return content
}

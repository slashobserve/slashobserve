import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import type { ResolvedLink } from "../../field-config"
import type { Cell } from "../table-model"
import { MoreLinks } from "./more-links"
import { ValueContent } from "./value-content"

export function TextCell({
  cell,
  links,
  tinted,
}: {
  cell: Cell
  links: ResolvedLink[]
  tinted: boolean
}) {
  const [first, ...more] = links
  const style = tinted && cell.color ? { color: cell.color } : undefined
  const content = <ValueContent cell={cell} />
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
        {more.length > 0 ? <MoreLinks links={more} /> : null}
      </span>
    )
  }
  if (style) {
    return (
      <span className="inline-flex items-center gap-1" style={style}>
        {content}
      </span>
    )
  }
  return content
}

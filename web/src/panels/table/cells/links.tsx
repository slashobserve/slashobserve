import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import type { ResolvedLink } from "../../field-config"
import type { Cell } from "../table-model"
import { ValueContent } from "./value-content"

export function LinksCell({
  cell,
  links,
}: {
  cell: Cell
  links: ResolvedLink[]
}) {
  if (links.length === 0) return <ValueContent cell={cell} />
  return (
    <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
      {links.map((link, index) => (
        <a
          key={`${index}:${link.href}`}
          {...anchorProps(link)}
          className="inline-flex items-center gap-1 underline decoration-border underline-offset-4 hover:decoration-current"
        >
          {link.title}
          {link.newTab ? <NewTabMark /> : null}
        </a>
      ))}
    </span>
  )
}

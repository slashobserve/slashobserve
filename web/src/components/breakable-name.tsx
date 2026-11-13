import { Fragment } from "react"

import { nameParts } from "@/lib/text"

export function BreakableName({ name }: { name: string }) {
  return nameParts(name).map((part, index) => (
    <Fragment key={index}>
      {index > 0 ? <wbr /> : null}
      {part}
    </Fragment>
  ))
}

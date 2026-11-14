import type { ReactNode } from "react"

import { cn } from "@/lib/utils"
import { useCrumbs, useHideCrumbWhenHeadingRepeats } from "@/shell/crumbs"
import { headingInPhoneBar } from "@/shell/phone-title"

type Props = {
  title: string
  description?: ReactNode
  actions?: ReactNode
  headingId?: string
}

export function PageHeader({ title, description, actions, headingId }: Props) {
  const { crumbs } = useCrumbs()
  const inBar = headingInPhoneBar(title, crumbs)
  const crumbRepeats = useHideCrumbWhenHeadingRepeats(title)
  const underCrumbs = crumbs.length > 0 && !crumbRepeats
  return (
    <div
      className={cn(
        "mb-4 flex flex-wrap items-end justify-between gap-x-6 gap-y-3",
        underCrumbs && "-mt-2 narrow:mt-0",
        inBar && !description && !actions && "narrow:sr-only"
      )}
    >
      <div className="flex min-w-0 flex-col">
        <h1
          id={headingId}
          className={cn(
            "text-xl/8 font-semibold tracking-[-0.01em]",
            inBar && "narrow:sr-only"
          )}
        >
          {title}
        </h1>
        {description ? (
          <p className="text-[13px] text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      ) : null}
    </div>
  )
}

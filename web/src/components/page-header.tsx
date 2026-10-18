import type { ReactNode } from "react"

type Props = {
  title: string
  description?: ReactNode
  actions?: ReactNode
  headingId?: string
}

export function PageHeader({ title, description, actions, headingId }: Props) {
  return (
    <div className="mb-4 flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
      <div className="flex min-w-0 flex-col gap-[3px]">
        <h1 id={headingId} className="text-xl font-semibold tracking-[-0.01em]">
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

import { useId, type ReactNode } from "react"

export function SettingsSection({
  title,
  description,
  action,
  children,
}: {
  title: string
  description: string
  action?: ReactNode
  children: ReactNode
}) {
  const headingId = useId()
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-3">
        <div>
          <h2 id={headingId} className="text-xl leading-7 font-semibold">
            {title}
          </h2>
          <p className="mt-1 max-w-[640px] text-[13px] text-muted-foreground">
            {description}
          </p>
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

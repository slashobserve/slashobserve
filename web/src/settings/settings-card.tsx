import { useId, type ComponentProps, type ReactNode } from "react"

import { CopyButton } from "@/components/copy-button"
import { LoadProblem } from "@/components/load-problem"
import { cn } from "@/lib/utils"

export function SettingsCard({
  id,
  title,
  hint,
  children,
}: {
  id?: string
  title: string
  hint: string
  children: ReactNode
}) {
  const headingId = useId()
  return (
    <section
      id={id}
      aria-labelledby={headingId}
      className="mb-4 max-w-[860px] rounded-card border bg-background"
    >
      <div className="border-b px-4 py-3.5">
        <h2 id={headingId} className="text-sm font-semibold">
          {title}
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
      </div>
      {children}
    </section>
  )
}

export function SettingsRow({
  label,
  children,
  copy,
  mono,
}: {
  label: string
  children: ReactNode
  copy?: string | null
  mono?: boolean
}) {
  return (
    <div
      data-slot="settings-row"
      className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-3 last:border-b-0"
    >
      <span className="w-[200px] text-[13px] text-muted-foreground">
        {label}
      </span>
      <span
        className={cn(
          "min-w-[200px] flex-1 text-[13px] wrap-anywhere",
          mono && "font-mono"
        )}
      >
        {children}
      </span>
      {copy === undefined ? null : copy === null ? (
        <span aria-hidden="true" className="size-7 flex-none" />
      ) : (
        <CopyButton
          text={copy}
          label={`Copy ${label}`}
          variant="outline"
          size="icon-sm"
        />
      )}
    </div>
  )
}

export function SettingsProblem(props: ComponentProps<typeof LoadProblem>) {
  return (
    <div className="p-4">
      <LoadProblem {...props} />
    </div>
  )
}

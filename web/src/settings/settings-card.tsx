import { useId, type ComponentProps, type ReactNode } from "react"
import { LockIcon } from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { LoadProblem } from "@/components/load-problem"
import { cn } from "@/lib/utils"

export function SettingsCard({
  title,
  hint,
  action,
  className,
  children,
}: {
  title: string
  hint: string
  action?: ReactNode
  className?: string
  children?: ReactNode
}) {
  const headingId = useId()
  return (
    <section
      aria-labelledby={headingId}
      className={cn(
        "overflow-hidden rounded-card border bg-card [&>:nth-child(2)]:border-t",
        className
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3.5">
        <div>
          <h3 id={headingId} className="text-sm font-semibold">
            {title}
          </h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

export function EnvChip({ name }: { name: string }) {
  return (
    <span className="inline-flex min-h-[22px] max-w-full flex-none items-center gap-1 rounded-chip bg-muted px-1.5 text-xs text-muted-foreground">
      <LockIcon aria-hidden="true" className="size-3 flex-none" />
      Set by <span className="font-mono break-all">{name}</span>
    </span>
  )
}

export function SettingsRow({
  label,
  children,
  copy,
  mono,
  note,
}: {
  label: string
  children: ReactNode
  copy?: string | null
  mono?: boolean
  note?: ReactNode
}) {
  return (
    <div
      data-slot="settings-row"
      className="flex min-h-7 flex-wrap items-center gap-x-4 gap-y-1 border-t px-4 py-2.5"
    >
      <span className="w-[200px] flex-none text-[13px] text-muted-foreground narrow:w-full">
        {label}
      </span>
      <span
        className={cn(
          "min-w-[180px] flex-1 text-[13px] wrap-anywhere",
          mono && "font-mono"
        )}
      >
        {children}
      </span>
      {note === undefined ? null : (
        <div className="flex narrow:order-last narrow:basis-full">{note}</div>
      )}
      {copy === undefined ? null : copy === null ? (
        <span aria-hidden="true" className="ml-auto size-7 flex-none" />
      ) : (
        <CopyButton
          text={copy}
          label={`Copy ${label}`}
          variant="outline"
          size="icon-sm"
          className="ml-auto narrow:size-11"
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

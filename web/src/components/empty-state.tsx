import type { ReactNode } from "react"
import { CircleCheckIcon } from "lucide-react"

type EmptyStateProps = {
  title: string
  children?: ReactNode
  action?: ReactNode
  icon?: ReactNode
  tone?: "default" | "ok"
}

export function EmptyState({
  title,
  children,
  action,
  icon,
  tone = "default",
}: EmptyStateProps) {
  const shownIcon =
    icon ??
    (tone === "ok" ? (
      <CircleCheckIcon aria-hidden className="size-5 text-status-ok" />
    ) : null)
  return (
    <div className="flex flex-col items-center gap-1.5 px-6 py-10 text-center">
      {shownIcon ? (
        <span className="text-muted-foreground [&_svg]:size-5">
          {shownIcon}
        </span>
      ) : null}
      <p className="text-[13px] font-medium">{title}</p>
      {children ? (
        <div className="max-w-[44ch] text-[13px] text-muted-foreground">
          {children}
        </div>
      ) : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}

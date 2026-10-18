import type { ComponentProps, ReactNode } from "react"
import { cn } from "cn"

type TableCardProps = {
  minWidth: number | string
  label?: string
  labelledBy?: string
  children: ReactNode
  className?: string
}

export function TableCard({
  minWidth,
  label,
  labelledBy,
  children,
  className,
}: TableCardProps) {
  return (
    <div
      role="region"
      tabIndex={0}
      aria-label={label}
      aria-labelledby={labelledBy}
      className={cn(
        "overflow-x-auto rounded-card border bg-background outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className
      )}
    >
      <table
        className="w-full border-collapse text-[13px]"
        style={{ minWidth }}
      >
        {children}
      </table>
    </div>
  )
}

export function HeaderCell({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      scope="col"
      className={cn(
        "border-b px-3.5 py-[9px] text-left font-medium whitespace-nowrap text-muted-foreground",
        className
      )}
      {...props}
    />
  )
}

export function Cell({ className, ...props }: ComponentProps<"td">) {
  return (
    <td
      className={cn(
        "border-b px-3.5 py-[11px] align-middle [tr:last-child_&]:border-b-0",
        className
      )}
      {...props}
    />
  )
}

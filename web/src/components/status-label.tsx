import type { ReactNode } from "react"

export type StatusTone = "ok" | "warning" | "critical" | "running" | "none"

type Props = {
  tone: StatusTone
  children: ReactNode
  title?: string
}

const iconProps = {
  width: 13,
  height: 13,
  viewBox: "0 0 24 24",
  fill: "none",
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
} as const

function StatusIcon({ tone }: { tone: StatusTone }) {
  switch (tone) {
    case "ok":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-ok"
          strokeWidth={2.4}
        >
          <path d="m5 12 5 5 9-10" />
        </svg>
      )
    case "warning":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-warning"
          strokeWidth={2.2}
        >
          <path d="M12 3 2 20h20L12 3z" />
          <path d="M12 10v4M12 17h.01" />
        </svg>
      )
    case "critical":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-critical"
          strokeWidth={2.2}
        >
          <circle cx={12} cy={12} r={9} />
          <path d="M12 8v5M12 16h.01" />
        </svg>
      )
    case "running":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-brand motion-safe:animate-spin"
          strokeWidth={2.4}
        >
          <path d="M21 12a9 9 0 1 1-6.2-8.6" />
        </svg>
      )
    case "none":
      return null
  }
}

export function StatusLabel({ tone, children, title }: Props) {
  return (
    <span
      data-status={tone}
      title={title}
      className="inline-flex items-center gap-1.5"
    >
      <StatusIcon tone={tone} />
      {children}
    </span>
  )
}

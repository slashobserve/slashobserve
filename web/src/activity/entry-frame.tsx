import type { ComponentProps, ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { ArrowRightIcon } from "lucide-react"

import { AgentMark } from "@/components/agent-mark"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { cn } from "@/lib/utils"
import { lowerFirst } from "@/observe/conversation"
import { sentence } from "@/observe/queries"

import { initialOf } from "./initial"

export type EntryAvatar = { kind: "person"; name: string } | { kind: "agent" }

type Props = {
  avatar: EntryAvatar
  who: string
  verb: string
  panel?: { title: string; onSelect: () => void }
  at: string
  nested?: boolean
  children?: ReactNode
}

export const boxClass = "rounded-[8px] border px-[11px] py-[9px] text-[13px]"

export function EntryFrame({
  avatar,
  who,
  verb,
  panel,
  at,
  nested,
  children,
}: Props) {
  const size = nested ? "size-[19px]" : "size-[23px]"
  return (
    <li className={cn("relative flex gap-2.5", nested ? "py-1.5" : "py-2")}>
      {nested ? null : (
        <span
          aria-hidden="true"
          className="absolute inset-y-0 left-[11px] w-px bg-border"
        />
      )}
      <span
        aria-hidden="true"
        className={cn(
          "relative grid flex-none place-items-center rounded-full border text-[11px] font-semibold",
          size,
          avatar.kind === "agent"
            ? "bg-primary text-primary-foreground"
            : "bg-background text-foreground"
        )}
      >
        {avatar.kind === "agent" ? (
          <AgentMark size={nested ? 10 : 12} />
        ) : (
          initialOf(avatar.name)
        )}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-[5px] pt-0.5">
        <p className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1 text-[13px]">
          <span className="font-medium wrap-anywhere">{who}</span>
          <span className="text-muted-foreground">{verb}</span>
          {panel === undefined ? null : (
            <button
              type="button"
              className="rounded-sm text-left font-medium wrap-anywhere underline decoration-border underline-offset-3 outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={panel.onSelect}
            >
              {panel.title}
            </button>
          )}
          <span className="text-xs text-muted-foreground">
            <RelativeTime iso={at} />
          </span>
        </p>
        {children}
      </div>
    </li>
  )
}

export function EntryBody({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        boxClass,
        "leading-normal wrap-anywhere whitespace-pre-wrap",
        className
      )}
      {...props}
    />
  )
}

export function EntryMeta({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
      {children}
    </div>
  )
}

export function MetaState({
  icon,
  children,
}: {
  icon?: ReactNode
  children: ReactNode
}) {
  return (
    <span className="inline-flex items-center gap-[5px]">
      {icon}
      {children}
    </span>
  )
}

export function SentToAgent() {
  return (
    <MetaState icon={<ArrowRightIcon className="size-3" />}>
      Sent to the agent
    </MetaState>
  )
}

export function ResolvedState({ text }: { text: string }) {
  return (
    <MetaState
      icon={
        <svg
          width={12}
          height={12}
          viewBox="0 0 24 24"
          fill="none"
          strokeWidth={2.4}
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="flex-none stroke-status-ok"
        >
          <path d="m5 12 5 5 9-10" />
        </svg>
      }
    >
      {text}
    </MetaState>
  )
}

export function RequestFailure({ error }: { error: string | null }) {
  const text =
    error === null || error.trim() === ""
      ? "The agent could not finish."
      : `The agent could not finish: ${lowerFirst(sentence(error))}`
  return (
    <StatusLabel tone="critical" title={text}>
      <span className="max-w-[320px] truncate">{text}</span>
    </StatusLabel>
  )
}

export function DetailsLink({
  requestId,
  label,
}: {
  requestId: string
  label: string
}) {
  return (
    <Link
      to="/observe/$requestId"
      params={{ requestId }}
      target="_blank"
      rel="noopener"
      aria-label={label}
      className="self-start text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
    >
      Details
    </Link>
  )
}

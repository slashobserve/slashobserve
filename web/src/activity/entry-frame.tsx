import type { ComponentProps, ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import {
  ArrowUpRightIcon,
  CheckIcon,
  SaveIcon,
  SparkleIcon,
} from "lucide-react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { cn } from "@/lib/utils"
import { lowerFirst } from "@/observe/conversation"
import { sentence } from "@/observe/queries"

import { initialOf } from "./initial"

export type EntryAvatar =
  { kind: "person"; name: string } | { kind: "agent" } | { kind: "version" }

type Props = {
  avatar: EntryAvatar
  who: string
  verb: string
  panel?: { title: string; onSelect: () => void }
  at: string
  nested?: boolean
  children?: ReactNode
}

function AvatarMark({ avatar }: { avatar: EntryAvatar }) {
  switch (avatar.kind) {
    case "person":
      return <>{initialOf(avatar.name)}</>
    case "agent":
      return <SparkleIcon className="size-3" />
    case "version":
      return <SaveIcon className="size-3" />
  }
}

export function EntryFrame({
  avatar,
  who,
  verb,
  panel,
  at,
  nested,
  children,
}: Props) {
  return (
    <li
      className={cn(
        "grid gap-2.5",
        nested ? "grid-cols-[19px_1fr]" : "grid-cols-[23px_1fr]"
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "flex items-center justify-center rounded-full border bg-background text-[11px] font-medium text-muted-foreground",
          nested ? "size-[19px]" : "size-[23px]"
        )}
      >
        <AvatarMark avatar={avatar} />
      </span>
      <div className="min-w-0">
        <p className="text-[13px] leading-[23px]">
          <span className="font-semibold wrap-anywhere">{who}</span>{" "}
          <span className="text-muted-foreground">{verb}</span>
          {panel === undefined ? null : (
            <>
              {" "}
              <button
                type="button"
                className="rounded-sm text-left wrap-anywhere text-foreground outline-none hover:underline focus-visible:underline focus-visible:ring-2 focus-visible:ring-ring"
                onClick={panel.onSelect}
              >
                {panel.title}
              </button>
            </>
          )}
          <span className="text-xs text-muted-foreground">
            {" · "}
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
        "mt-1.5 rounded-control border px-2.5 py-2 text-[13px] leading-normal wrap-anywhere whitespace-pre-wrap",
        className
      )}
      {...props}
    />
  )
}

export function EntryMeta({ children }: { children: ReactNode }) {
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
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
    <span className="inline-flex items-center gap-1.5">
      {icon}
      {children}
    </span>
  )
}

export function SentToAgent() {
  return (
    <MetaState icon={<ArrowUpRightIcon className="size-3" />}>
      Sent to the agent
    </MetaState>
  )
}

export function ResolvedState({ text }: { text: string }) {
  return (
    <MetaState icon={<CheckIcon className="size-3 text-status-ok" />}>
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
      className="mt-1 inline-block rounded-sm text-xs text-muted-foreground outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
    >
      Details
    </Link>
  )
}

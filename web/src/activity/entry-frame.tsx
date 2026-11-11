import type { ComponentProps, ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { ArrowRightIcon } from "lucide-react"

import { AgentMark } from "@/components/agent-mark"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import type { Comment } from "@/comments/queries"
import { isAgentActor } from "@/lib/origin"
import { lowerFirst } from "@/lib/text"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { initialOf } from "./initial"

type EntryAvatar =
  | { kind: "person"; name: string }
  | { kind: "service"; name: string }
  | { kind: "agent" }

export function avatarFor(author: {
  kind: "user" | "service_account"
  name: string
}): EntryAvatar {
  return author.kind === "service_account"
    ? { kind: "service", name: author.name }
    : { kind: "person", name: author.name }
}

export function commentAvatar(comment: Pick<Comment, "actor" | "author">) {
  return isAgentActor(comment.actor)
    ? { kind: "agent" as const }
    : avatarFor(comment.author)
}

type Props = {
  avatar: EntryAvatar
  who: string
  verb: string
  panel?: { title: string; onSelect: () => void }
  at: string
  edited?: boolean
  nested?: boolean
  children?: ReactNode
}

export const boxClass = "rounded-control border px-2.5 py-2 text-[13px]"

export const panelLinkClass =
  "rounded-sm text-left font-medium wrap-anywhere text-foreground underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"

const avatarShape: Record<EntryAvatar["kind"], string> = {
  person: "rounded-full border bg-background text-foreground",
  service: "rounded-control border bg-muted text-foreground",
  agent: "rounded-control bg-primary text-primary-foreground",
}

export function EntryFrame({
  avatar,
  who,
  verb,
  panel,
  at,
  edited,
  nested,
  children,
}: Props) {
  return (
    <li
      className={cn(
        "group/entry relative flex gap-2.5",
        nested ? "py-1.5" : "py-2"
      )}
    >
      {nested ? null : (
        <span
          aria-hidden="true"
          className="absolute top-[35px] bottom-0 left-[11px] w-px bg-border group-last/entry:hidden"
        />
      )}
      <span
        aria-hidden="true"
        className={cn(
          "relative grid flex-none place-items-center text-[11px] font-semibold",
          nested ? "size-[19px] text-[10px]" : "size-[23px]",
          avatarShape[avatar.kind]
        )}
      >
        {avatar.kind === "agent" ? (
          <AgentMark size={nested ? 10 : 12} />
        ) : (
          initialOf(avatar.name)
        )}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1 pt-0.5">
        <div className="flex items-baseline gap-2 text-[13px]">
          <p className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-1.5 gap-y-1">
            <span className="font-medium wrap-anywhere text-foreground">
              {who}
            </span>
            <span className="text-muted-foreground">{verb}</span>
            {panel === undefined ? null : (
              <button
                type="button"
                className={panelLinkClass}
                onClick={panel.onSelect}
              >
                {panel.title}
              </button>
            )}
          </p>
          <span className="ml-auto flex-none pl-2 text-xs text-muted-foreground tabular-nums">
            {edited ? "edited · " : null}
            <RelativeTime iso={at} />
          </span>
        </div>
        {children}
      </div>
    </li>
  )
}

export function EntryBody({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "text-[13px] leading-relaxed wrap-anywhere whitespace-pre-wrap text-foreground",
        className
      )}
      {...props}
    />
  )
}

export function EntryMeta({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {children}
    </div>
  )
}

const entryActionClass = "text-muted-foreground hover:text-foreground"

export function EntryAction({
  icon,
  children,
  className,
  ...props
}: Omit<ComponentProps<typeof Button>, "variant" | "size"> & {
  icon: ReactNode
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="xs"
      className={cn(entryActionClass, "first:-ml-2", className)}
      {...props}
    >
      {icon}
      {children}
    </Button>
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
      to="/c/$requestId"
      params={{ requestId }}
      target="_blank"
      rel="noopener"
      aria-label={label}
      className={cn(
        buttonVariants({ variant: "ghost", size: "xs" }),
        entryActionClass,
        "-ml-2 self-start"
      )}
    >
      Details
    </Link>
  )
}

import { useEffect, useRef, type ComponentProps, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { ArrowRightIcon } from "lucide-react"

import { sentence } from "@/api/errors"
import { AuthorMark } from "@/components/author-mark"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import type { AuthorMarkModel } from "@/lib/author"
import { lowerFirst } from "@/lib/text"
import { cn } from "@/lib/utils"

import { EntryTime } from "./entry-time"

type Props = {
  avatar: AuthorMarkModel
  who: string
  verb?: string | undefined
  panel?: { title: string; onSelect?: (() => void) | undefined } | undefined
  at: string
  edited?: boolean
  nested?: boolean
  focused?: boolean | undefined
  unread?: { onRead: () => void } | undefined
  className?: string | undefined
  children?: ReactNode
}

export const boxClass = "rounded-control border px-2.5 py-2 text-[13px]"

export const panelLinkClass =
  "rounded-sm text-left font-medium wrap-anywhere text-foreground underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"

export function EntryFrame({
  avatar,
  who,
  verb,
  panel,
  at,
  edited,
  nested,
  focused,
  unread,
  className,
  children,
}: Props) {
  const item = useRef<HTMLLIElement>(null)
  useEffect(() => {
    if (focused) item.current?.scrollIntoView({ block: "nearest" })
  }, [focused])
  return (
    <li
      ref={item}
      aria-current={focused ? "true" : undefined}
      onClickCapture={unread?.onRead}
      onFocusCapture={unread?.onRead}
      className={cn(
        "group/entry relative flex gap-2.5",
        nested ? "py-1.5" : "py-2",
        focused && "rounded-control bg-selection",
        className
      )}
    >
      {nested ? null : (
        <span
          aria-hidden="true"
          className="absolute top-[34px] bottom-0 left-[11px] w-px bg-border group-last/entry:hidden"
        />
      )}
      <AuthorMark mark={avatar} size={nested ? 20 : 24} />
      <div className="flex min-w-0 flex-1 flex-col gap-1 pt-0.5">
        <div className="flex items-baseline gap-2 text-[13px]">
          <p className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-1.5 gap-y-1">
            {unread === undefined ? null : (
              <button
                type="button"
                aria-label="Unread, mark as read"
                title="Unread, mark as read"
                onClick={unread.onRead}
                className="size-2 flex-none self-center rounded-full bg-primary outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            )}
            <span className="font-medium wrap-anywhere text-foreground">
              {who}
            </span>
            {verb === undefined ? null : (
              <span className="text-muted-foreground">{verb}</span>
            )}
            {panel === undefined ? null : panel.onSelect === undefined ? (
              <span className="font-medium wrap-anywhere text-foreground">
                {panel.title}
              </span>
            ) : (
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
            <EntryTime iso={at} />
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

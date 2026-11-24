import { useEffect, useRef, useState } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { EllipsisIcon, PencilIcon, Trash2Icon } from "lucide-react"

import { EntryBody } from "@/activity/entry-frame"
import type { components } from "@/api/schema.gen"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

import { chatAnswerLink } from "./chat-answer-link"
import { DeleteMessageDialog } from "./delete-message-dialog"
import { EditMessageForm } from "./edit-message-form"
import { MessageBody } from "./message-body"
import type { MentionPerson } from "./mentions"

type Mention = components["schemas"]["Mention"]

function writtenByPerson(actor: components["schemas"]["Actor"]): boolean {
  return (
    actor.kind === "user" && actor.run_id === null && actor.origin !== "mcp"
  )
}

export function MessageText({
  message,
  people,
  target,
  queryKey,
  root = false,
  folded = false,
  bodyHidden = false,
}: {
  message: {
    id: string
    body: string
    actor: components["schemas"]["Actor"]
    mentions: readonly Mention[]
    can_edit: boolean
    can_delete: boolean
  }
  people: readonly MentionPerson[]
  target: string | null
  queryKey: QueryKey
  root?: boolean
  folded?: boolean
  bodyHidden?: boolean
}) {
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const menuButton = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)

  useEffect(() => {
    if (editing || !refocus.current) return
    refocus.current = false
    menuButton.current?.focus()
  }, [editing])

  if (editing && !folded) {
    return (
      <EditMessageForm
        message={message}
        people={people}
        target={target}
        queryKey={queryKey}
        onClose={() => {
          refocus.current = true
          setEditing(false)
        }}
      />
    )
  }
  const markdown = !folded && writtenByPerson(message.actor)
  const codeSpans = !folded && !markdown
  const chat = folded
    ? null
    : chatAnswerLink(message.body, message.actor.run_id)
  const menu = (message.can_edit || message.can_delete) && !folded
  if (bodyHidden && !menu) return null
  return (
    <div className="flex items-start gap-1">
      {bodyHidden ? (
        <span className="flex-1" />
      ) : (
        <EntryBody
          className={cn(
            "min-w-0 flex-1",
            markdown && "whitespace-normal",
            folded && "line-clamp-1 text-muted-foreground"
          )}
        >
          <MessageBody
            body={chat === null ? message.body : chat.before}
            mentions={message.mentions}
            markdown={markdown}
            codeSpans={codeSpans}
          />
          {chat === null ? null : (
            <Link
              to="/c/$requestId"
              params={{ requestId: chat.runId }}
              className="underline underline-offset-2"
            >
              {chat.label}
            </Link>
          )}
        </EntryBody>
      )}
      {menu ? (
        <>
          <DropdownMenu>
            <DropdownMenuTrigger
              ref={menuButton}
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  aria-label={root ? "Thread actions" : "Comment actions"}
                  className="-mt-0.5 flex-none text-muted-foreground opacity-0 group-focus-within/entry:opacity-100 group-hover/entry:opacity-100 hover:text-foreground focus-visible:opacity-100 data-popup-open:opacity-100 motion-safe:transition-opacity motion-safe:duration-100 [@media(hover:none)]:opacity-100"
                />
              }
            >
              <EllipsisIcon aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {message.can_edit ? (
                <DropdownMenuItem onClick={() => setEditing(true)}>
                  <PencilIcon aria-hidden="true" />
                  Edit
                </DropdownMenuItem>
              ) : null}
              {message.can_delete ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setDeleting(true)}
                >
                  <Trash2Icon aria-hidden="true" />
                  Delete
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
          <DeleteMessageDialog
            open={deleting}
            onOpenChange={setDeleting}
            messageId={message.id}
            wholeThread={root}
            queryKey={queryKey}
            finalFocus={menuButton}
          />
        </>
      ) : null}
    </div>
  )
}

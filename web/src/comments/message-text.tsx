import { useEffect, useRef, useState } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PencilIcon } from "lucide-react"

import { EntryAction, EntryBody } from "@/activity/entry-frame"
import type { components } from "@/api/schema.gen"
import { cn } from "@/lib/utils"

import { chatAnswerLink } from "./chat-answer-link"
import { DeleteMessageButton } from "./delete-message-button"
import { EditMessageForm } from "./edit-message-form"
import { MessageBody } from "./message-body"
import type { MentionPerson } from "./mentions"

type Mention = components["schemas"]["Mention"]

export function MessageText({
  message,
  people,
  target,
  queryKey,
  root = false,
  folded = false,
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
}) {
  const [editing, setEditing] = useState(false)
  const editButton = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)

  useEffect(() => {
    if (editing || !refocus.current) return
    refocus.current = false
    editButton.current?.focus()
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
  const runAuthored = message.actor.run_id !== null
  const markdown = !folded && !runAuthored
  const codeSpans = !folded && runAuthored
  const chat = folded
    ? null
    : chatAnswerLink(message.body, message.actor.run_id)
  return (
    <>
      <EntryBody
        className={cn(
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
      {(message.can_edit || message.can_delete) && !folded ? (
        <div className="flex items-center">
          {message.can_edit ? (
            <EntryAction
              ref={editButton}
              icon={<PencilIcon aria-hidden="true" />}
              onClick={() => setEditing(true)}
            >
              Edit
            </EntryAction>
          ) : null}
          {message.can_delete ? (
            <DeleteMessageButton
              messageId={message.id}
              wholeThread={root}
              queryKey={queryKey}
            />
          ) : null}
        </div>
      ) : null}
    </>
  )
}

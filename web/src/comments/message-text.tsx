import { useEffect, useRef, useState } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { PencilIcon } from "lucide-react"

import { EntryAction, EntryBody } from "@/activity/entry-frame"
import type { components } from "@/api/schema.gen"
import { cn } from "@/lib/utils"

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
  return (
    <>
      <EntryBody className={cn(folded && "line-clamp-1 text-muted-foreground")}>
        <MessageBody body={message.body} mentions={message.mentions} />
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

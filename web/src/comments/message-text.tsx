import { useEffect, useRef, useState } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { PencilIcon } from "lucide-react"

import { EntryAction, EntryBody } from "@/activity/entry-frame"
import type { components } from "@/api/schema.gen"
import { cn } from "@/lib/utils"

import { EditMessageForm } from "./edit-message-form"
import { MessageBody } from "./message-body"
import type { MentionPerson } from "./mentions"

type Mention = components["schemas"]["Mention"]

export function MessageText({
  message,
  people,
  queryKey,
  folded = false,
}: {
  message: {
    id: string
    body: string
    mentions: readonly Mention[]
    can_edit: boolean
  }
  people: readonly MentionPerson[]
  queryKey: QueryKey
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
      {message.can_edit && !folded ? (
        <div className="flex items-center">
          <EntryAction
            ref={editButton}
            icon={<PencilIcon aria-hidden="true" />}
            onClick={() => setEditing(true)}
          >
            Edit
          </EntryAction>
        </div>
      ) : null}
    </>
  )
}

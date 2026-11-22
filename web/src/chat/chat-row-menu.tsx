import { useState, type FormEvent, type RefObject } from "react"
import { useQueries } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import {
  EllipsisIcon,
  LoaderCircleIcon,
  PencilIcon,
  PinIcon,
  PinOffIcon,
  Trash2Icon,
} from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  sentence,
  useDeleteChatTurns,
  useMyRequests,
  usePinChat,
  useRenameChat,
  type RequestSummary,
} from "@/observe/queries"
import { observationQuery, useDeleteObservation } from "@/observations/queries"

import {
  deleteObservationLabel,
  deleteTurnsText,
  savedObservations,
} from "./chat-state"
import { useArchiveChat } from "./chats-queries"
import { keptTitleNote } from "./shared-title"

const maxTitleLength = 120
const chatTurnsShown = 50
const archiveText = "The chat leaves your chat lists."

type ChatRef = Pick<RequestSummary, "id" | "title" | "pinned_at">

function failureText(error: unknown): string {
  return error instanceof Error && error.message !== ""
    ? sentence(error.message)
    : "The chat could not be changed. Try again."
}

export function chatRowId(requestId: string): string {
  return `chat-row-${requestId}`
}

function focusRow(requestId: string) {
  requestAnimationFrame(() =>
    document.getElementById(chatRowId(requestId))?.focus()
  )
}

export function ChatRowMenu({
  request,
  turns: givenTurns,
  wholeChat = false,
  current,
  onRename,
  afterDelete,
  className,
}: {
  request: ChatRef
  turns?: readonly RequestSummary[]
  wholeChat?: boolean
  current: boolean
  onRename: () => void
  afterDelete: RefObject<HTMLElement | null>
  className?: string
}) {
  const navigate = useNavigate()
  const pin = usePinChat(request.id)
  const [confirming, setConfirming] = useState(false)
  const loaded = useMyRequests(
    { chatId: request.id },
    chatTurnsShown,
    wholeChat && confirming
  )
  const turns = wholeChat ? loaded.requests : (givenTurns ?? [])
  const removeTurns = useDeleteChatTurns(turns.map((turn) => turn.id))
  const archive = useArchiveChat(request.id)
  const remove = wholeChat ? archive : removeTurns
  const removeObservation = useDeleteObservation()
  const [problem, setProblem] = useState<string | null>(null)
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set())
  const pinned = request.pinned_at !== null
  const saved = savedObservations(turns)
  const observations = useQueries({
    queries: saved.map((observation) => ({
      ...observationQuery(observation.id),
      enabled: confirming,
    })),
  })
  const deletable = saved.filter(
    (_, index) =>
      observations[index]?.data?.observation.allowed_actions.includes(
        "delete"
      ) === true
  )
  const removing = new Set(
    deletable
      .filter((observation) => chosen.has(observation.id))
      .map((observation) => observation.id)
  )
  const pending = remove.isPending || removeObservation.isPending

  function changeConfirming(next: boolean) {
    if (!next && pending) return
    setConfirming(next)
    if (!next) {
      setProblem(null)
      setChosen(new Set())
    }
  }

  function choose(id: string, on: boolean) {
    setChosen((current) => {
      const next = new Set(current)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
  }

  function togglePin() {
    pin.mutate(!pinned, {
      onError: (error) => toast.error(failureText(error)),
    })
  }

  async function confirmDelete() {
    setProblem(null)
    for (const id of removing) {
      try {
        await removeObservation.mutateAsync(id)
      } catch (error) {
        setProblem(
          `The observation could not be deleted, so the chat was kept: ${failureText(error)}`
        )
        return
      }
    }
    try {
      await remove.mutateAsync(undefined)
    } catch (error) {
      setProblem(failureText(error))
      return
    }
    setConfirming(false)
    afterDelete.current?.focus()
    if (current) void navigate({ to: "/ask" })
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Chat actions: ${request.title}`}
              className={className}
            />
          }
        >
          <EllipsisIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-max">
          <DropdownMenuItem onClick={onRename}>
            <PencilIcon aria-hidden="true" />
            Rename
          </DropdownMenuItem>
          <DropdownMenuItem onClick={togglePin}>
            {pinned ? (
              <PinOffIcon aria-hidden="true" />
            ) : (
              <PinIcon aria-hidden="true" />
            )}
            {pinned ? "Unpin" : "Pin"}
          </DropdownMenuItem>
          <DropdownMenuItem
            variant="destructive"
            onClick={() => setConfirming(true)}
          >
            <Trash2Icon aria-hidden="true" />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog open={confirming} onOpenChange={changeConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this chat?</AlertDialogTitle>
            <AlertDialogDescription>
              {turns.length === 0
                ? archiveText
                : deleteTurnsText(turns, removing)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deletable.length > 0 ? (
            <ul className="flex flex-col gap-2">
              {deletable.map((observation) => (
                <li key={observation.id}>
                  <label className="flex items-start gap-2 text-[13px]">
                    <Checkbox
                      className="mt-0.5"
                      checked={chosen.has(observation.id)}
                      disabled={pending}
                      onCheckedChange={(on) => choose(observation.id, on)}
                    />
                    <span className="wrap-anywhere">
                      {deleteObservationLabel(observation)}
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          ) : null}
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={pending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={pending}
              focusableWhenDisabled
              aria-busy={pending}
              onClick={() => void confirmDelete()}
            >
              {pending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              {removing.size === 0
                ? "Delete chat"
                : removing.size === 1
                  ? "Delete chat and observation"
                  : "Delete chat and observations"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export function ChatRenameForm({
  request,
  rowId = request.id,
  onDone,
}: {
  request: ChatRef
  rowId?: string
  onDone: () => void
}) {
  const rename = useRenameChat(request.id)
  const [title, setTitle] = useState(request.title)

  function finish() {
    onDone()
    focusRow(rowId)
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    if (rename.isPending) return
    if (title.trim() === request.title) {
      finish()
      return
    }
    rename.mutate(title, {
      onSuccess: (body) => {
        finish()
        const note = keptTitleNote("chat", body.request.observation_rename)
        if (note !== null) toast.info(note)
      },
      onError: (error) => toast.error(failureText(error)),
    })
  }

  return (
    <form onSubmit={onSubmit} className="flex-1 px-2 py-2">
      <Input
        aria-label="Chat title"
        maxLength={maxTitleLength}
        value={title}
        readOnly={rename.isPending}
        aria-busy={rename.isPending || undefined}
        autoFocus
        onFocus={(event) => event.currentTarget.select()}
        onChange={(event) => setTitle(event.target.value)}
        onKeyDown={(event) => {
          if (event.key !== "Escape") return
          event.preventDefault()
          event.stopPropagation()
          finish()
        }}
      />
    </form>
  )
}

import { useState, type FormEvent, type RefObject } from "react"
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  sentence,
  useDeleteChat,
  usePinChat,
  useRenameChat,
  type RequestSummary,
} from "@/observe/queries"

const maxTitleLength = 120

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
  current,
  onRename,
  afterDelete,
  className,
}: {
  request: RequestSummary
  current: boolean
  onRename: () => void
  afterDelete: RefObject<HTMLElement | null>
  className?: string
}) {
  const navigate = useNavigate()
  const pin = usePinChat(request.id)
  const remove = useDeleteChat(request.id)
  const [confirming, setConfirming] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const pinned = request.pinned_at !== null

  function changeConfirming(next: boolean) {
    if (!next && remove.isPending) return
    setConfirming(next)
    if (!next) setProblem(null)
  }

  function togglePin() {
    pin.mutate(!pinned, {
      onError: (error) => toast.error(failureText(error)),
    })
  }

  function confirmDelete() {
    setProblem(null)
    remove.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false)
        afterDelete.current?.focus()
        if (current) void navigate({ to: "/ask" })
      },
      onError: (error) => setProblem(failureText(error)),
    })
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
              It leaves your chat lists. The dashboard it made stays.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={confirmDelete}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete chat
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export function ChatRenameForm({
  request,
  onDone,
}: {
  request: RequestSummary
  onDone: () => void
}) {
  const rename = useRenameChat(request.id)
  const [title, setTitle] = useState(request.title)

  function finish() {
    onDone()
    focusRow(request.id)
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    if (rename.isPending) return
    if (title.trim() === request.title) {
      finish()
      return
    }
    rename.mutate(title, {
      onSuccess: finish,
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

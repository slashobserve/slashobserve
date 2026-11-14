import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react"
import { LinkIcon, MoreHorizontalIcon } from "lucide-react"
import { toast } from "sonner"

import { AuthorMark } from "@/components/author-mark"
import { DockToggle } from "@/components/dock/dock-toggle"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { markOf } from "@/lib/author"
import { observationPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { TopRowActions } from "@/shell/top-row-actions"

import { blockWriteProblem } from "./errors"
import { discussionToggleLabel } from "./discussion-changes"
import {
  useUpdateObservation,
  type Observation,
  type ObservationEnvelope,
} from "./queries"
import { StateControl } from "./state-control"

const titleType = "text-[28px] leading-9 font-semibold tracking-[-0.01em]"

type Props = {
  envelope: ObservationEnvelope
  discussion: {
    shown: boolean
    count: number
    onToggle: () => void
    toggleRef: RefObject<HTMLButtonElement | null>
  }
  onMove: () => void
  moreButtonRef: RefObject<HTMLButtonElement | null>
  extraActions?: ReactNode
}

function EditableTitle({ observation }: { observation: Observation }) {
  const canEdit = observation.allowed_actions.includes("update")
  const update = useUpdateObservation(observation.id)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(observation.title)
  const [problem, setProblem] = useState<string | null>(null)
  const settled = useRef(false)
  const returnFocus = useRef(false)
  const titleButton = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!editing && returnFocus.current) {
      returnFocus.current = false
      titleButton.current?.focus()
    }
  }, [editing])

  function begin() {
    settled.current = false
    setDraft(observation.title)
    setProblem(null)
    setEditing(true)
  }

  function leave(focus: boolean) {
    settled.current = true
    returnFocus.current = focus
    setProblem(null)
    setEditing(false)
  }

  function commit(focus: boolean) {
    if (settled.current || update.isPending) return
    const next = draft.trim()
    if (next === "" || next === observation.title) {
      leave(focus)
      return
    }
    update.mutate(
      { title: next },
      {
        onSuccess: () => leave(focus),
        onError: (error) => setProblem(blockWriteProblem(error).text),
      }
    )
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault()
      commit(true)
    } else if (event.key === "Escape") {
      event.preventDefault()
      event.stopPropagation()
      leave(true)
    }
  }

  if (!canEdit) return <h1 className={titleType}>{observation.title}</h1>

  if (editing) {
    return (
      <div className="flex flex-col gap-1">
        <h1 className="contents">
          <Input
            aria-label="Title"
            autoFocus
            maxLength={200}
            value={draft}
            readOnly={update.isPending}
            aria-invalid={problem !== null}
            className={cn(titleType, "h-11 md:text-[28px] narrow:text-[28px]")}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={onKeyDown}
            onBlur={() => commit(false)}
          />
        </h1>
        {problem === null ? null : (
          <p role="alert" className="text-sm text-destructive">
            {problem}
          </p>
        )}
      </div>
    )
  }

  return (
    <h1 className={titleType}>
      <button
        ref={titleButton}
        type="button"
        className="-mx-2 w-[calc(100%+1rem)] rounded-control px-2 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
        onClick={begin}
      >
        {observation.title}
      </button>
    </h1>
  )
}

async function copyLink(id: string) {
  try {
    await navigator.clipboard.writeText(
      window.location.origin + observationPath(id)
    )
    toast.success("Link copied")
  } catch {
    toast.error("Could not copy the link")
  }
}

export function ObservationHeader({
  envelope,
  discussion: { shown, count, onToggle, toggleRef },
  onMove,
  moreButtonRef,
  extraActions,
}: Props) {
  const { observation, folder } = envelope
  const canMove = observation.allowed_actions.includes("move")

  return (
    <>
      <TopRowActions>
        <AuthorMark mark={markOf(observation.created_by)} size={24} />
        {extraActions}
        <Button
          variant="outline"
          size="sm"
          onClick={() => void copyLink(observation.id)}
        >
          <LinkIcon aria-hidden="true" />
          Share
        </Button>
        <DockToggle
          ref={toggleRef}
          label={discussionToggleLabel(count)}
          count={count}
          pressed={shown}
          onToggle={onToggle}
        />
        {canMove ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  ref={moreButtonRef}
                  variant="ghost"
                  size="icon-sm"
                  aria-label="More actions"
                />
              }
            >
              <MoreHorizontalIcon aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={onMove}>
                Move to folder
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </TopRowActions>
      <div className="flex flex-col gap-3">
        <EditableTitle observation={observation} />
        <StateControl observation={observation} folder={folder} />
      </div>
    </>
  )
}

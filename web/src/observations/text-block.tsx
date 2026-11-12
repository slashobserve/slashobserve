import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react"
import { PencilIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { FindingMarkdown } from "@/findings/finding-markdown"
import { isModEnter } from "@/lib/shortcuts"

import { blockWriteProblem } from "./errors"
import { useUpdateBlock, type Block } from "./queries"

const emptyProblem = "Write something, or delete the block"
const conflictProblem = "Someone changed this block while you were editing"

function markdownOf(block: Block): string {
  return typeof block.body.markdown === "string" ? block.body.markdown : ""
}

type Problem = { text: string; theirs: Block | null }

function TextEditor({ block, onClose }: { block: Block; onClose: () => void }) {
  const problemId = useId()
  const update = useUpdateBlock(block.observation_id)
  const [text, setText] = useState(markdownOf(block))
  const [version, setVersion] = useState(block.version)
  const [problem, setProblem] = useState<Problem | null>(null)
  const field = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const element = field.current
    if (element === null) return
    element.focus()
    element.setSelectionRange(element.value.length, element.value.length)
  }, [])

  function save() {
    if (update.isPending) return
    if (text.trim() === "") {
      setProblem({ text: emptyProblem, theirs: null })
      return
    }
    setProblem(null)
    update.mutate(
      { blockId: block.id, version, body: { markdown: text } },
      {
        onSuccess: onClose,
        onError: (error) => {
          const failure = blockWriteProblem(error)
          setProblem(
            failure.kind === "conflict"
              ? { text: conflictProblem, theirs: failure.current }
              : { text: failure.text, theirs: null }
          )
        },
      }
    )
  }

  function takeTheirs(theirs: Block) {
    setText(markdownOf(theirs))
    setVersion(theirs.version)
    setProblem(null)
    field.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isModEnter(event)) {
      event.preventDefault()
      save()
    } else if (event.key === "Escape") {
      event.stopPropagation()
      onClose()
    }
  }

  const theirs = problem?.theirs ?? null

  return (
    <div className="flex flex-col gap-2">
      <Textarea
        ref={field}
        aria-label="Text block"
        aria-invalid={problem !== null}
        aria-describedby={problem === null ? undefined : problemId}
        className="min-h-24"
        value={text}
        readOnly={update.isPending}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={onKeyDown}
      />
      {problem === null ? null : (
        <div className="flex flex-wrap items-center gap-2">
          <p id={problemId} role="alert" className="text-sm text-destructive">
            {problem.text}
          </p>
          {theirs === null ? null : (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => takeTheirs(theirs)}
            >
              Use their version
            </Button>
          )}
        </div>
      )}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          focusableWhenDisabled
          disabled={update.isPending}
          aria-busy={update.isPending}
          onClick={save}
        >
          Save
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

export function TextBlock({ block }: { block: Block }) {
  const canEdit = block.allowed_actions.includes("update")
  const [editing, setEditing] = useState(false)
  const editButton = useRef<HTMLButtonElement>(null)
  const returnFocus = useRef(false)

  useEffect(() => {
    if (!editing && returnFocus.current) {
      returnFocus.current = false
      editButton.current?.focus()
    }
  }, [editing])

  function close() {
    returnFocus.current = true
    setEditing(false)
  }

  if (editing) return <TextEditor block={block} onClose={close} />

  return (
    <div className="relative">
      <FindingMarkdown markdown={markdownOf(block)} />
      {canEdit ? (
        <Button
          ref={editButton}
          variant="outline"
          size="sm"
          className="absolute -top-3 right-9 z-10 opacity-0 group-focus-within/block:opacity-100 group-hover/block:opacity-100 focus-visible:opacity-100"
          onClick={() => setEditing(true)}
        >
          <PencilIcon aria-hidden="true" />
          Edit
        </Button>
      ) : null}
    </div>
  )
}

import { useId, useRef, useState, type KeyboardEvent } from "react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Textarea } from "@/components/ui/textarea"
import { isModEnter } from "@/lib/shortcuts"

import { addProblemText, blockTitle } from "./block-rows"
import { PanelPicker } from "./panel-picker"
import { useAddBlock, type Block, type Observation } from "./queries"

export const addBlockRowId = "add-block-row"

export function useFreezeBlock(
  observationId: string,
  handlers: {
    onAdded: (blockId: string) => void
    onProblem: (text: string) => void
  }
) {
  const add = useAddBlock(observationId)
  return {
    pending: add.isPending,
    freeze(source: Block) {
      if (add.isPending) return
      add.mutate(
        {
          kind: "snapshot",
          from_block_id: source.id,
          position: source.position + 1,
        },
        {
          onSuccess: ({ block }) => handlers.onAdded(block.id),
          onError: (error) => handlers.onProblem(addProblemText(error)),
        }
      )
    },
  }
}

type Props = {
  observation: Observation
  blocks: readonly Block[]
  canEdit: boolean
  onAdded: (blockId: string) => void
}

function TextComposer({
  observation,
  verb,
  onAdded,
  onClose,
}: {
  observation: Observation
  verb: string
  onAdded: (blockId: string) => void
  onClose: () => void
}) {
  const problemId = useId()
  const addBlock = useAddBlock(observation.id)
  const [text, setText] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const empty = text.trim() === ""

  function submit() {
    if (empty || addBlock.isPending) return
    setProblem(null)
    addBlock.mutate(
      { kind: "text", body: { markdown: text } },
      {
        onSuccess: ({ block }) => {
          onAdded(block.id)
          onClose()
        },
        onError: (error) => setProblem(addProblemText(error)),
      }
    )
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isModEnter(event)) {
      event.preventDefault()
      submit()
    } else if (event.key === "Escape") {
      event.stopPropagation()
      onClose()
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <Textarea
        aria-label="New text block"
        aria-describedby={problem === null ? undefined : problemId}
        aria-invalid={problem !== null}
        autoFocus
        className="min-h-24"
        placeholder="Write Markdown"
        value={text}
        readOnly={addBlock.isPending}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={onKeyDown}
      />
      {problem === null ? null : (
        <p id={problemId} role="alert" className="text-sm text-destructive">
          {problem}
        </p>
      )}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          focusableWhenDisabled
          disabled={empty || addBlock.isPending}
          aria-busy={addBlock.isPending}
          onClick={submit}
        >
          {verb}
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

export function AddBlockRow({ observation, blocks, canEdit, onAdded }: Props) {
  const label = canEdit ? "Add a block" : "Suggest a block"
  const verb = canEdit ? "Add" : "Suggest"
  const textButton = useRef<HTMLButtonElement>(null)
  const panelButton = useRef<HTMLButtonElement>(null)
  const [composing, setComposing] = useState(false)
  const [picking, setPicking] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const freezable = blocks.filter(
    (block) => block.kind === "panel" && block.state === "live"
  )
  const freeze = useFreezeBlock(observation.id, {
    onAdded,
    onProblem: setProblem,
  })

  function closeComposer() {
    setComposing(false)
    textButton.current?.focus()
  }

  return (
    <div className="flex flex-col gap-3">
      <div
        id={addBlockRowId}
        role="group"
        aria-label={label}
        tabIndex={-1}
        className="flex flex-wrap items-center gap-2 outline-none"
      >
        <span aria-hidden="true" className="text-xs text-muted-foreground">
          {label}
        </span>
        <Button
          ref={textButton}
          variant="outline"
          size="sm"
          aria-expanded={composing}
          onClick={() => {
            setProblem(null)
            setComposing(true)
          }}
        >
          Text
        </Button>
        <Button
          ref={panelButton}
          variant="outline"
          size="sm"
          aria-expanded={picking}
          onClick={() => {
            setProblem(null)
            setPicking(true)
          }}
        >
          Panel
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                focusableWhenDisabled
                disabled={freezable.length === 0 || freeze.pending}
                aria-busy={freeze.pending}
              />
            }
          >
            Snapshot
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-48">
            {freezable.map((block) => (
              <DropdownMenuItem
                key={block.id}
                onClick={() => {
                  setProblem(null)
                  freeze.freeze(block)
                }}
              >
                {blockTitle(block)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {problem === null ? null : (
        <p role="alert" className="text-sm text-destructive">
          {problem}
        </p>
      )}
      {composing ? (
        <TextComposer
          observation={observation}
          verb={verb}
          onAdded={onAdded}
          onClose={closeComposer}
        />
      ) : null}
      {picking ? (
        <PanelPicker
          observation={observation}
          verb={verb}
          trigger={panelButton}
          onPicked={onAdded}
          onClose={() => setPicking(false)}
        />
      ) : null}
    </div>
  )
}

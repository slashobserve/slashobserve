import { useId, useRef, useState, type KeyboardEvent } from "react"
import { useNavigate } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Textarea } from "@/components/ui/textarea"
import { useDatasources } from "@/datasources/queries"
import { isModEnter } from "@/lib/shortcuts"

import {
  addPanelTriggerId,
  newQueryPanel,
  useBlockEditor,
} from "./block-editor"
import { addProblemText, blockTitle } from "./block-rows"
import { PanelPicker } from "./panel-picker"
import { useAddBlock, type Block, type Observation } from "./queries"

export const addBlockRowId = "add-block-row"

const dataPages = [
  { label: "Logs", to: "/logs" },
  { label: "Traces", to: "/traces" },
  { label: "Metrics", to: "/explore/metrics" },
] as const

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
  picking: boolean
  onPickingChange: (picking: boolean) => void
  onAdded: (blockId: string) => void
}

function TextComposer({
  observation,
  verb,
  text,
  onText,
  onAdded,
  onClose,
}: {
  observation: Observation
  verb: string
  text: string
  onText: (text: string) => void
  onAdded: (blockId: string) => void
  onClose: (discard: boolean) => void
}) {
  const problemId = useId()
  const addBlock = useAddBlock(observation.id)
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
          onClose(true)
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
      onClose(false)
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
        placeholder="Write Markdown, then press Cmd or Ctrl and Enter to add it"
        value={text}
        readOnly={addBlock.isPending}
        onChange={(event) => onText(event.target.value)}
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
        <Button variant="ghost" size="sm" onClick={() => onClose(true)}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

export function AddBlockRow({
  observation,
  blocks,
  canEdit,
  picking,
  onPickingChange,
  onAdded,
}: Props) {
  const label = canEdit ? "Add a block" : "Suggest a block"
  const verb = canEdit ? "Add" : "Suggest"
  const navigate = useNavigate()
  const textButton = useRef<HTMLButtonElement>(null)
  const panelButton = useRef<HTMLButtonElement>(null)
  const [composing, setComposing] = useState(false)
  const [draft, setDraft] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const freezable = blocks.filter(
    (block) => block.kind === "panel" && block.state === "live"
  )
  const freeze = useFreezeBlock(observation.id, {
    onAdded,
    onProblem: setProblem,
  })
  const editor = useBlockEditor()
  const datasources = useDatasources()
  const newPanel = newQueryPanel(datasources.data ?? [])

  function closeComposer(discard: boolean) {
    if (discard) setDraft("")
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
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                ref={panelButton}
                id={addPanelTriggerId}
                variant="outline"
                size="sm"
              />
            }
          >
            Panel
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-48">
            <DropdownMenuItem
              onClick={() => {
                setProblem(null)
                onPickingChange(true)
              }}
            >
              From a dashboard
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={editor === null || newPanel === null}
              onClick={() => {
                if (editor === null || newPanel === null) return
                setProblem(null)
                editor.open({ kind: "new", panel: newPanel })
              }}
            >
              New query
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {dataPages.map((page) => (
          <Button
            key={page.label}
            variant="outline"
            size="sm"
            onClick={() =>
              void navigate({
                to: page.to,
                search: { observation: observation.id },
              })
            }
          >
            {page.label}
          </Button>
        ))}
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
      {observation.state !== "resolved" ? null : (
        <p className="text-xs text-muted-foreground">
          {canEdit
            ? "This observation is resolved. Blocks you add still go on the page."
            : "This observation is resolved. A block you suggest waits for an editor to keep it."}
        </p>
      )}
      {problem === null ? null : (
        <p role="alert" className="text-sm text-destructive">
          {problem}
        </p>
      )}
      {composing ? (
        <TextComposer
          observation={observation}
          verb={verb}
          text={draft}
          onText={setDraft}
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
          onClose={() => onPickingChange(false)}
        />
      ) : null}
    </div>
  )
}

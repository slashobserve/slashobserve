import { useId, useState, type RefObject } from "react"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Skeleton } from "@/components/ui/skeleton"
import { observationLink } from "@/lib/links"
import { sentence } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"
import { panelTitle, type DataPanel } from "@/spec"

import { addProblemText, newPanelBody } from "./block-rows"
import { FolderSelect } from "./folder-select"
import { blockPanelFrom, blockRange } from "./panel-input"
import {
  observationQuery,
  observationsInfiniteQuery,
  useAddBlockTo,
  useCreateObservation,
  type Observation,
} from "./queries"

type Props = {
  panel: DataPanel
  from?: string
  to?: string
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
  defaultObservationId?: string
}

type Where = "open" | "new"

const maxTitleLength = 200
const emptyListText = "No open observations. Start a new one."

function problemText(error: unknown): string {
  if (error instanceof ApiError && error.status === 403) {
    return sentence(error.message)
  }
  return addProblemText(error)
}

function useOpenObservations(defaultObservationId: string | undefined) {
  const list = useInfiniteQuery(observationsInfiniteQuery({ state: "open" }))
  const firstPage = list.data?.pages[0]?.observations
  const listed = firstPage?.some((item) => item.id === defaultObservationId)
  const preset = useQuery({
    ...observationQuery(defaultObservationId ?? ""),
    enabled:
      defaultObservationId !== undefined && firstPage !== undefined && !listed,
  })
  const presetObservation =
    preset.data?.observation.state === "open"
      ? preset.data.observation
      : undefined
  const items: Observation[] | undefined =
    firstPage === undefined
      ? undefined
      : presetObservation === undefined || listed
        ? firstPage
        : [presetObservation, ...firstPage]
  return { items, error: list.isError ? list.error : null, retry: list.refetch }
}

export function AddToObservationPanel({
  panel,
  from,
  to,
  onClose,
  trigger,
  defaultObservationId,
}: Props) {
  const ids = useId()
  const navigate = useNavigate()
  const open = useOpenObservations(defaultObservationId)
  const createObservation = useCreateObservation()
  const addBlock = useAddBlockTo()
  const [where, setWhere] = useState<Where>("open")
  const [selectedId, setSelectedId] = useState<string | null>(
    defaultObservationId ?? null
  )
  const [title, setTitle] = useState(panelTitle(panel))
  const [folderId, setFolderId] = useState<string | null>(null)
  const [created, setCreated] = useState<Observation | null>(null)
  const [problem, setProblem] = useState<string | null>(null)

  const noOpen = open.items !== undefined && open.items.length === 0
  const mode: Where = noOpen ? "new" : where
  const selected = open.items?.find((item) => item.id === selectedId) ?? null
  const pending = createObservation.isPending || addBlock.isPending
  const ready = mode === "open" ? selected !== null : title.trim() !== ""

  function addTo(target: Observation) {
    const body = newPanelBody(
      blockPanelFrom(panel),
      blockRange(from, to, Date.now())
    )
    addBlock.mutate(
      { observationId: target.id, kind: "panel", body },
      {
        onSuccess: ({ block }) => {
          const verb = block.state === "live" ? "Added to" : "Suggested on"
          toast.success(`${verb} ${target.title}`, {
            action: {
              label: "Open",
              onClick: () => void navigate(observationLink(target.id)),
            },
          })
          onClose()
        },
        onError: (error) => setProblem(problemText(error)),
      }
    )
  }

  function submit() {
    if (pending || !ready) return
    setProblem(null)
    if (mode === "open") {
      if (selected !== null) addTo(selected)
      return
    }
    if (created !== null) {
      addTo(created)
      return
    }
    createObservation.mutate(
      {
        title: title.trim(),
        ...(folderId === null ? {} : { folder_id: folderId }),
      },
      {
        onSuccess: (envelope) => {
          setCreated(envelope.observation)
          addTo(envelope.observation)
        },
        onError: (error) => setProblem(loadProblemText(error)),
      }
    )
  }

  let openField
  if (open.error !== null) {
    openField = (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-col items-start gap-2">
          {loadProblemText(open.error)}
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => void open.retry()}
          >
            Retry
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (open.items === undefined) {
    openField = <Skeleton aria-busy="true" className="h-8" />
  } else {
    openField = (
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-observation`}>Observation</Label>
        <Combobox<Observation>
          items={open.items}
          value={selected}
          disabled={pending}
          itemToStringLabel={(item) => item.title}
          isItemEqualToValue={(item, value) => item.id === value.id}
          onValueChange={(next) => setSelectedId(next?.id ?? null)}
        >
          <ComboboxInput
            id={`${ids}-observation`}
            disabled={pending}
            placeholder="Search observations"
          />
          <ComboboxContent>
            <ComboboxEmpty>No observations found</ComboboxEmpty>
            <ComboboxList>
              {(item: Observation) => (
                <ComboboxItem key={item.id} value={item}>
                  <span className="min-w-0 flex-1 truncate">{item.title}</span>
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
    )
  }

  const newFields = (
    <>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-title`}>Title</Label>
        <Input
          id={`${ids}-title`}
          value={title}
          maxLength={maxTitleLength}
          autoComplete="off"
          required
          readOnly={pending || created !== null}
          onChange={(event) => setTitle(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-folder`}>Folder</Label>
        <FolderSelect
          id={`${ids}-folder`}
          value={folderId}
          disabled={pending || created !== null}
          onChange={setFolderId}
        />
      </div>
    </>
  )

  return (
    <SidePanel
      title="Add to observation"
      description="The panel stays live on the observation, over the range you are viewing when it has one."
      trigger={trigger}
      closeDisabled={pending}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            focusableWhenDisabled
            disabled={!ready || pending}
            aria-busy={pending}
            onClick={submit}
          >
            {pending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="size-3.5 motion-safe:animate-spin"
              />
            ) : null}
            Add
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2">
        <p id={`${ids}-where`} className="text-[13px] font-medium">
          Where
        </p>
        <RadioGroup
          aria-labelledby={`${ids}-where`}
          value={mode}
          disabled={pending || created !== null}
          onValueChange={(next) => setWhere(next as Where)}
        >
          <label className="flex items-center gap-2 text-[13px]">
            <RadioGroupItem value="open" disabled={noOpen} />
            An open observation
          </label>
          <label className="flex items-center gap-2 text-[13px]">
            <RadioGroupItem value="new" />A new observation
          </label>
        </RadioGroup>
        {noOpen ? (
          <p className="text-[13px] text-muted-foreground">{emptyListText}</p>
        ) : null}
      </div>
      {mode === "open" ? openField : newFields}
      {problem === null ? null : (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col gap-1">
            <p>{problem}</p>
            {created === null ? null : (
              <p>{`The observation "${created.title}" was created. Add again to put the panel on it.`}</p>
            )}
          </AlertDescription>
        </Alert>
      )}
    </SidePanel>
  )
}

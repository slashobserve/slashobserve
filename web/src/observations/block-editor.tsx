import {
  createContext,
  lazy,
  Suspense,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react"
import { createPortal } from "react-dom"

import { useActivityStore, usePanelLayout } from "@/activity/activity-store"
import { EditorBoundary } from "@/activity/editor-boundary"
import { EditorViewSwitch } from "@/activity/editor-view-switch"
import type { DraftLocation } from "@/activity/leaves-draft"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { isEditablePanel, type EditablePanel } from "@/panel-editor/draft"
import { useDraftTitle } from "@/panel-editor/draft-title"
import { useLeaveGuard } from "@/panel-editor/leave-guard"
import type { BlockTarget, EditorView } from "@/panel-editor/panel-editor"
import { storedBlockPanel } from "@/panel-editor/queries"
import { newPanelDraft } from "@/panel-editor/new-panel"
import { guardLoad } from "@/panels/renderer-load"
import { PanelHostContext } from "@/shell/side-panel"
import {
  panelTitle,
  type Dashboard,
  type DataPanel,
  type Variable,
} from "@/spec"

import {
  blockPanelId,
  PageRangeContext,
  panelBlockBody,
  resolveBlockRange,
} from "./blocks/block-data"
import { blockVariableStates } from "./panel-input"
import {
  useAddBlock,
  useUpdateBlock,
  type Block,
  type Observation,
} from "./queries"

const PanelEditor = lazy(guardLoad(() => import("@/panel-editor/panel-editor")))

export type BlockEditRequest =
  { kind: "edit"; blockId: string } | { kind: "new"; panel: DataPanel }

type BlockEditor = {
  open: (request: BlockEditRequest) => void
  canSuggest: boolean
}

export const BlockEditorContext = createContext<BlockEditor | null>(null)

export function useBlockEditor(): BlockEditor | null {
  return useContext(BlockEditorContext)
}

export const addPanelTriggerId = "add-block-panel"

const suggestionOutcome = "An editor keeps or discards your suggestion"
const blockGrid = { x: 0, y: 0, w: 24, h: 8 }
const noChoices = {}

type Leave = "close" | "saved" | "replace"

export type ReplaceGuard = (replace: () => void) => void

function discardFocus(kind: Leave | null) {
  return kind === "replace" ? ("none" as const) : ("trigger" as const)
}

export type EditableBlock = {
  blockId: string | null
  version: number
  panel: EditablePanel
  variables: Variable[]
  range: { from: string; to: string }
}

export function editableBlock(
  block: Block | undefined,
  page: { from: string | null; to: string | null }
): EditableBlock | null {
  if (block === undefined || block.kind !== "panel") return null
  const body = panelBlockBody(block)
  if (body === null || !isEditablePanel(body.panel)) return null
  const range = resolveBlockRange(page, body)
  return {
    blockId: block.id,
    version: block.version,
    panel: { ...body.panel, gridPos: blockGrid },
    variables: body.variables ?? [],
    range: { from: range.from, to: range.to },
  }
}

export function newBlock(
  panel: DataPanel,
  page: { from: string | null; to: string | null }
): EditableBlock {
  const range = resolveBlockRange(page, {})
  return {
    blockId: null,
    version: 0,
    panel: { ...panel, id: blockPanelId, gridPos: blockGrid },
    variables: [],
    range: { from: range.from, to: range.to },
  }
}

const emptyPage: Dashboard = {
  specVersion: "0",
  title: "",
  time: { from: "now-1h", to: "now" },
  panels: [],
}

export function newQueryPanel(
  datasources: readonly { uid: string }[]
): DataPanel | null {
  return newPanelDraft(emptyPage, datasources)
}

export function blockSpec(title: string, editable: EditableBlock): Dashboard {
  return {
    specVersion: "0",
    title,
    time: editable.range,
    variables: editable.variables,
    panels: [editable.panel],
  } as Dashboard
}

function leavesObservation(
  current: DraftLocation,
  next: DraftLocation
): boolean {
  return (
    current.routeId !== next.routeId ||
    current.params.observationId !== next.params.observationId
  )
}

function triggerOf(request: BlockEditRequest): HTMLElement | null {
  if (request.kind === "new") return document.getElementById(addPanelTriggerId)
  const section = document.querySelector<HTMLElement>(
    `[data-block-id="${CSS.escape(request.blockId)}"]`
  )
  return section?.querySelector<HTMLElement>("[data-panel-menu]") ?? section
}

function EditorSkeleton() {
  return (
    <div className="flex flex-col gap-4 px-4 py-4" aria-busy="true">
      <Skeleton className="h-8 w-full animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-40 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}

type Props = {
  hidden: boolean
  observation: Observation
  blocks: readonly Block[]
  request: BlockEditRequest
  canEdit: boolean
  onClose: () => void
  onSaved: (blockId: string) => void
  replaceGuardRef: RefObject<ReplaceGuard | null>
}

export function BlockEditorDock(props: Props) {
  const host = useContext(PanelHostContext)
  if (host === null) return null
  return createPortal(<BlockEditorPane {...props} />, host)
}

type SaveMode = "add" | "update" | "suggest"

function saveModeOf(block: Block | undefined): SaveMode {
  if (block === undefined) return "add"
  return block.allowed_actions.includes("update") ? "update" : "suggest"
}

function useBlockSave(
  observationId: string,
  block: Block | undefined,
  deletedBlock: Block | null,
  editable: EditableBlock | null,
  canEdit: boolean,
  savedId: { current: string | null }
): BlockTarget | null {
  const update = useUpdateBlock(observationId)
  const add = useAddBlock(observationId)
  if (editable === null) return null
  const base = {
    kind: "block" as const,
    observationId,
    blockId: editable.blockId,
  }
  if (deletedBlock !== null) {
    return {
      ...base,
      saveLabel: canEdit ? "Add as a new block" : "Suggest as a new block",
      outcome: canEdit ? null : suggestionOutcome,
      save: (panel) =>
        add
          .mutateAsync({
            kind: "panel",
            body: { ...deletedBlock.body, panel: storedBlockPanel(panel) },
            width: deletedBlock.width,
            position: deletedBlock.position,
          })
          .then(({ block: added }) => {
            savedId.current = added.id
          }),
    }
  }
  if (block === undefined) {
    return {
      ...base,
      saveLabel: canEdit ? "Add" : "Suggest",
      outcome: canEdit ? null : suggestionOutcome,
      save: (panel) =>
        add
          .mutateAsync({
            kind: "panel",
            body: { panel: storedBlockPanel(panel) },
          })
          .then(({ block: added }) => {
            savedId.current = added.id
          }),
    }
  }
  if (saveModeOf(block) === "update") {
    return {
      ...base,
      saveLabel: "Save",
      outcome: null,
      save: (panel) =>
        update
          .mutateAsync({
            blockId: block.id,
            version: block.version,
            body: { ...block.body, panel: storedBlockPanel(panel) },
          })
          .then(() => {
            savedId.current = block.id
          }),
    }
  }
  return {
    ...base,
    saveLabel: "Suggest edit",
    outcome: suggestionOutcome,
    save: (panel) =>
      add
        .mutateAsync({
          kind: "panel",
          body: { ...block.body, panel: storedBlockPanel(panel) },
          width: block.width,
          position: block.position + 1,
        })
        .then(({ block: added }) => {
          savedId.current = added.id
        }),
  }
}

function BlockEditorPane({
  hidden,
  observation,
  blocks,
  request,
  canEdit,
  onClose,
  onSaved,
  replaceGuardRef,
}: Props) {
  const headingId = useId()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const page = useContext(PageRangeContext)
  const layout = usePanelLayout()
  const setPanelWidth = useActivityStore((state) => state.setPanelWidth)
  const [view, setView] = useState<EditorView>("editor")
  const returnFocus = useRef(false)
  const savedId = useRef<string | null>(null)
  const draftTitle = useDraftTitle(blockPanelId)
  const block =
    request.kind === "edit"
      ? blocks.find((entry) => entry.id === request.blockId)
      : undefined
  const [newPanel] = useState(() =>
    request.kind === "new" ? newBlock(request.panel, page) : null
  )
  const current = useMemo(
    () => newPanel ?? editableBlock(block, page),
    [newPanel, block, page]
  )
  const [retained, setRetained] = useState(current)
  if (current !== null && current !== retained) setRetained(current)
  const [lastBlock, setLastBlock] = useState(block)
  if (block !== undefined && block !== lastBlock) setLastBlock(block)
  const pendingReplace = useRef<(() => void) | null>(null)

  const onLeave = useCallback(
    (kind: Leave) => {
      if (kind === "replace") {
        pendingReplace.current?.()
        return
      }
      if (kind === "saved" && savedId.current !== null) {
        onSaved(savedId.current)
        return
      }
      returnFocus.current = true
      onClose()
    },
    [onClose, onSaved]
  )
  const {
    root,
    dirty,
    onDirtyChange,
    viewGuard,
    focusFirstQuery,
    requestView,
    leave,
    run,
    onKeyDown,
    dialog,
  } = useLeaveGuard<Leave>({
    view,
    onViewChange: setView,
    closeKind: "close",
    leaves: leavesObservation,
    onLeave,
    trigger: () => triggerOf(request),
    discardFocus,
  })
  const editable = current ?? (dirty ? retained : null)
  const deleted = request.kind === "edit" && block === undefined
  const deletedBlock = deleted && editable !== null ? (lastBlock ?? null) : null
  const target = useBlockSave(
    observation.id,
    block,
    deletedBlock,
    editable,
    canEdit,
    savedId
  )
  const mode = saveModeOf(block ?? lastBlock)

  useEffect(() => {
    const guard: ReplaceGuard = (replace) => {
      pendingReplace.current = replace
      leave("replace")
    }
    replaceGuardRef.current = guard
    return () => {
      if (replaceGuardRef.current === guard) replaceGuardRef.current = null
    }
  }, [leave, replaceGuardRef])
  const spec = useMemo(
    () => (editable === null ? null : blockSpec(observation.title, editable)),
    [editable, observation.title]
  )
  const selections = useMemo(
    () => blockVariableStates(editable?.variables ?? [], noChoices).selections,
    [editable?.variables]
  )

  useEffect(
    () => () => {
      if (!returnFocus.current) return
      requestAnimationFrame(() => triggerOf(request)?.focus())
    },
    [request]
  )

  let body: ReactNode
  if (editable === null || target === null || spec === null) {
    body = (
      <div className="flex flex-col items-start gap-3 px-4 py-4">
        <p className="text-[13px] text-muted-foreground">
          {deleted
            ? "Someone deleted this block."
            : "This block cannot be edited."}
        </p>
        <Button variant="outline" onClick={() => leave("close")}>
          Close
        </Button>
      </div>
    )
  } else {
    body = (
      <EditorBoundary>
        <Suspense fallback={<EditorSkeleton />}>
          <PanelEditor
            target={target}
            spec={spec}
            panel={editable.panel}
            version={editable.version}
            readOnly={false}
            view={view}
            focusFirstQuery={focusFirstQuery}
            onViewChange={setView}
            viewGuardRef={viewGuard}
            timeRange={editable.range}
            selections={selections}
            onDirtyChange={onDirtyChange}
            onSaved={() => run("saved")}
            onOpenLatest={() => {}}
            onDiscard={() => leave("close")}
          />
        </Suspense>
      </EditorBoundary>
    )
  }

  let title = "Block not found"
  if (request.kind === "new") title = canEdit ? "New query" : "Suggest a query"
  else if (editable !== null) {
    const name = draftTitle ?? panelTitle(editable.panel)
    title = mode === "suggest" ? `Suggest an edit to ${name}` : `Edit ${name}`
  }

  return (
    <DockPanel
      panelKey="block-editor"
      hidden={hidden}
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setPanelWidth}
      stacked={layout.stacked}
    >
      <div
        ref={root}
        className="flex min-h-0 flex-1 flex-col"
        onKeyDown={onKeyDown}
      >
        <DockHeader
          headingId={headingId}
          headingRef={headingRef}
          title={title}
          description={
            target?.outcome === suggestionOutcome
              ? "A draft. An editor keeps or discards it."
              : "A draft. Nothing changes until you save."
          }
          controls={
            editable === null ? null : (
              <EditorViewSwitch view={view} onChange={requestView} />
            )
          }
          onClose={() => leave("close")}
          closeLabel="Close panel editor"
        />
        {deletedBlock === null ? null : (
          <Alert variant="destructive" className="mx-4 mt-3 w-auto">
            <AlertDescription>
              Someone deleted this block. Your draft stays here: add it as a new
              block, or discard it.
            </AlertDescription>
          </Alert>
        )}
        <div className="flex min-h-0 flex-1 flex-col">{body}</div>
        {dialog}
      </div>
    </DockPanel>
  )
}

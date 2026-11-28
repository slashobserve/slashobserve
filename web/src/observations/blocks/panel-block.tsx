import {
  useContext,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useContainerWidth } from "react-grid-layout"
import {
  CalendarClockIcon,
  PencilIcon,
  PinIcon,
  PinOffIcon,
} from "lucide-react"
import { toast } from "sonner"

import type { Selections } from "@/api/keys"
import { PanelChrome } from "@/dashboards/panel-chrome"
import { PanelContainer } from "@/dashboards/panel-container"
import type { VariableState } from "@/dashboards/use-variables"
import { VariableBox } from "@/dashboards/variable-controls/variable-box"
import { VariablesBar } from "@/dashboards/variables-bar"
import type { BlockMenuHost, MenuItem } from "@/dashboards/panel-menu"
import type { PanelSource } from "@/dashboards/panel-source"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useDatasources } from "@/datasources/queries"
import { createSharedCursor } from "@/panels/shared-cursor"
import { useUpdateBlock, type Block } from "@/observations/queries"
import { isDataPanel, isRowPanel, panelTitle, type LayoutPanel } from "@/spec"

import { useBlockEditor } from "../block-editor"
import { blockWriteProblem } from "../errors"
import { blockVariableStates, variableCaption } from "../panel-input"
import { BlockByline } from "./block-byline"
import {
  blockPanelId,
  datasourceUidOf,
  PageRangeContext,
  panelBlockBody,
  pinnedBody,
  resolveBlockRange,
  type BlockWindow,
  type PanelBlockBody,
} from "./block-data"
import { useBlockInspect } from "./block-inspect"

const unreadablePanel = {
  id: blockPanelId,
  type: "text",
  title: "Panel",
  content: "",
} as unknown as LayoutPanel

const noAnnotations: never[] = []

const noUnverified = new Map<string, string>()

function ignore() {}

type Props = { block: Block; height: number; onCanvas?: boolean }

function RangeCaption({
  datasource,
  range,
  variables,
}: {
  datasource: string | undefined
  range: BlockWindow
  variables: string
}) {
  const live = range.refreshMs !== null
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-[12px] text-muted-foreground">
      <span
        aria-hidden="true"
        className={
          live
            ? "size-1.5 flex-none rounded-full bg-status-ok"
            : "size-1.5 flex-none rounded-full bg-muted-foreground"
        }
      />
      <span className="truncate">
        {[datasource ?? "", range.caption, variables]
          .filter((part) => part !== "")
          .join(", ")}
      </span>
    </span>
  )
}

function StoredVariable({ state }: { state: VariableState }) {
  const id = useId()
  return (
    <VariableBox variable={state.variable} id={id}>
      <output id={id} className="flex items-center px-[9px]">
        {state.selected.length === 0 ? "None" : state.selected.join(", ")}
      </output>
    </VariableBox>
  )
}

function StoredVariables({ states }: { states: VariableState[] }) {
  const shown = states.filter((state) => state.variable.hide !== "variable")
  if (shown.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-2 pr-2.5 pb-2.5 pl-4">
      {shown.map((state) => (
        <StoredVariable key={state.variable.name} state={state} />
      ))}
    </div>
  )
}

function BlockFrame({
  block,
  caption,
  children,
}: {
  block: Block
  caption?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex w-full flex-col gap-2">
      {children}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-0.5">
        {caption ?? <span />}
        <BlockByline block={block} />
      </div>
    </div>
  )
}

function ReadablePanelBlock({
  block,
  body,
  height,
  onCanvas,
}: {
  block: Block
  body: PanelBlockBody
  height: number
  onCanvas: boolean
}) {
  const { panel } = body
  const page = useContext(PageRangeContext)
  const range = useMemo(() => resolveBlockRange(page, body), [page, body])
  const variables = useMemo(() => body.variables ?? [], [body.variables])
  const [chosen, setChosen] = useState<Selections>({})
  const { states, fixed, selections } = useMemo(
    () => blockVariableStates(variables, chosen),
    [variables, chosen]
  )
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const [cursor] = useState(createSharedCursor)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const datasources = useDatasources()
  const inspect = useBlockInspect()
  const source = useMemo<PanelSource>(
    () => ({
      kind: "block",
      observationId: block.observation_id,
      blockId: block.id,
      version: block.version,
    }),
    [block.observation_id, block.id, block.version]
  )
  const update = useUpdateBlock(block.observation_id)
  const [choosingWindow, setChoosingWindow] = useState(false)
  const canPin = block.allowed_actions.includes("update")
  const editor = useBlockEditor()
  const canOpenEditor = editor !== null && (canPin || editor.canSuggest)

  function write(change: { body?: Block["body"]; follow_page?: boolean }) {
    if (update.isPending) return
    update.mutate(
      { blockId: block.id, version: block.version, ...change },
      { onError: (error) => toast.error(blockWriteProblem(error).text) }
    )
  }

  function pin(from: string, to: string) {
    const next = pinnedBody(block.body, from, to, Date.now())
    if (next !== null) write({ body: next })
  }

  const editActions: MenuItem[] = canOpenEditor
    ? [
        {
          id: "edit-panel",
          label: canPin ? "Edit panel" : "Suggest an edit",
          icon: <PencilIcon aria-hidden="true" />,
          onSelect: () => editor.open({ kind: "edit", blockId: block.id }),
        },
      ]
    : []
  const pinActions: MenuItem[] = canPin
    ? [
        ...(range.pinned
          ? []
          : [
              {
                id: "pin-now",
                label: "Pin to this window",
                icon: <PinIcon aria-hidden="true" />,
                disabled: update.isPending,
                onSelect: () => pin(range.from, range.to),
              },
            ]),
        {
          id: "pin-window",
          label: "Pin to a window",
          icon: <CalendarClockIcon aria-hidden="true" />,
          disabled: update.isPending,
          opensPopup: true,
          onSelect: () => setChoosingWindow(true),
        },
        ...(range.pinned
          ? [
              {
                id: "follow",
                label: "Follow the page",
                icon: <PinOffIcon aria-hidden="true" />,
                disabled: update.isPending,
                onSelect: () => write({ follow_page: true }),
              },
            ]
          : []),
      ]
    : []
  const blockMenu: BlockMenuHost = {
    ...(inspect === null ? {} : { onInspect: () => inspect.open(block.id) }),
    actions: [...editActions, ...pinActions],
  }
  const dataPanel = isDataPanel(panel) ? panel : undefined
  const datasourceUid = dataPanel ? datasourceUidOf(dataPanel) : undefined
  const datasourceName =
    datasourceUid === undefined
      ? undefined
      : (datasources.data?.find((entry) => entry.uid === datasourceUid)?.name ??
        datasourceUid)

  if (isRowPanel(panel)) return null
  const layoutPanel = panel as LayoutPanel

  return (
    <BlockFrame
      block={block}
      caption={
        choosingWindow ? (
          <TimeRangeControl
            value={{ from: range.from, to: range.to }}
            defaultOpen
            onDismiss={() => setChoosingWindow(false)}
            onChange={(next) => {
              setChoosingWindow(false)
              pin(next.from, next.to)
            }}
          />
        ) : (
          <RangeCaption
            datasource={datasourceName}
            range={range}
            variables={variableCaption(selections)}
          />
        )
      }
    >
      <VariablesBar
        states={states}
        unverified={noUnverified}
        onSelect={(name, values) =>
          setChosen((current) => ({ ...current, [name]: values }))
        }
      />
      <StoredVariables states={fixed} />
      <div ref={containerRef} style={{ height }} className="w-full">
        {mounted ? (
          <PanelContainer
            source={source}
            cursor={cursor}
            panel={layoutPanel}
            reused={undefined}
            variables={variables}
            title={panelTitle(layoutPanel)}
            repeatOf={undefined}
            width={width}
            height={height}
            from={range.from}
            to={range.to}
            timezone={undefined}
            rangeFrom={range.from}
            rangeTo={range.to}
            selections={selections}
            selectionsReady
            blockedBy={undefined}
            onRetryBlocked={ignore}
            refreshMs={range.refreshMs}
            mixedDatasources={false}
            editable={false}
            layoutSession={false}
            openComments={0}
            unverifiedReason={undefined}
            highlighted={false}
            editingPanel={false}
            canComment={onCanvas}
            onCanvas={onCanvas}
            canEdit={false}
            canPropose={false}
            focusOnRender={false}
            onFocused={ignore}
            composerRef={composerRef}
            annotations={noAnnotations}
            inspected={inspect?.inspectedId === block.id}
            onInspectTarget={inspect?.publish}
            blockMenu={blockMenu}
            onPinRange={
              canPin
                ? (fromMs, toMs) =>
                    pin(String(Math.round(fromMs)), String(Math.round(toMs)))
                : undefined
            }
          />
        ) : null}
      </div>
    </BlockFrame>
  )
}

export function PanelBlock({ block, height, onCanvas = false }: Props) {
  const body = useMemo(() => panelBlockBody(block), [block])
  if (body === null) {
    return (
      <BlockFrame block={block}>
        <div style={{ height }} className="w-full">
          <PanelChrome panel={unreadablePanel} state="ready">
            <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
              This block could not be read.
            </div>
          </PanelChrome>
        </div>
      </BlockFrame>
    )
  }
  return (
    <ReadablePanelBlock
      block={block}
      body={body}
      height={height}
      onCanvas={onCanvas}
    />
  )
}

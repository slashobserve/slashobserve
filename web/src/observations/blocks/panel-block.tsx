import { useMemo, useRef, useState, type ReactNode } from "react"
import { useContainerWidth } from "react-grid-layout"

import type { Selections } from "@/api/keys"
import { PanelChrome } from "@/dashboards/panel-chrome"
import { PanelContainer } from "@/dashboards/panel-container"
import { VariablesBar } from "@/dashboards/variables-bar"
import type { BlockMenuHost } from "@/dashboards/panel-menu"
import type { PanelSource } from "@/dashboards/panel-source"
import { useDatasources } from "@/datasources/queries"
import { createSharedCursor } from "@/panels/shared-cursor"
import type { Block } from "@/observations/queries"
import { isDataPanel, isRowPanel, panelTitle, type LayoutPanel } from "@/spec"

import { blockVariableStates, variableCaption } from "../panel-input"
import { BlockByline } from "./block-byline"
import {
  blockPanelId,
  datasourceUidOf,
  panelBlockBody,
  type PanelBlockBody,
} from "./block-data"
import { useBlockInspect } from "./block-inspect"

const defaultFrom = "now-1h"

const defaultTo = "now"

const unreadablePanel = {
  id: blockPanelId,
  type: "text",
  title: "Panel",
  content: "",
} as unknown as LayoutPanel

const noAnnotations: never[] = []

const noUnverified = new Map<string, string>()

function ignore() {}

type Props = { block: Block; height: number }

function LiveCaption({
  datasource,
  variables,
}: {
  datasource: string | undefined
  variables: string
}) {
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-[12px] text-muted-foreground">
      <span
        aria-hidden="true"
        className="size-1.5 flex-none rounded-full bg-status-ok"
      />
      <span className="truncate">
        {[datasource === undefined ? "live" : `${datasource}, live`, variables]
          .filter((part) => part !== "")
          .join(", ")}
      </span>
    </span>
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
}: {
  block: Block
  body: PanelBlockBody
  height: number
}) {
  const { panel, from, to } = body
  const variables = useMemo(() => body.variables ?? [], [body.variables])
  const [chosen, setChosen] = useState<Selections>({})
  const { states, selections } = useMemo(
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
  const open = inspect?.open
  const blockId = block.id
  const blockMenu = useMemo<BlockMenuHost>(
    () => ({
      onInspect: open === undefined ? undefined : () => open(blockId),
      range: { from, to },
      vars: selections,
    }),
    [open, blockId, from, to, selections]
  )
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
        <LiveCaption
          datasource={datasourceName}
          variables={variableCaption(selections)}
        />
      }
    >
      <VariablesBar
        states={states}
        unverified={noUnverified}
        onSelect={(name, values) =>
          setChosen((current) => ({ ...current, [name]: values }))
        }
      />
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
            from={from}
            to={to}
            timezone={undefined}
            rangeFrom={from ?? defaultFrom}
            rangeTo={to ?? defaultTo}
            selections={selections}
            selectionsReady
            blockedBy={undefined}
            onRetryBlocked={ignore}
            refreshMs={null}
            mixedDatasources={false}
            editable={false}
            layoutSession={false}
            openComments={0}
            unverifiedReason={undefined}
            highlighted={false}
            editingPanel={false}
            canComment={false}
            canEdit={false}
            focusOnRender={false}
            onFocused={ignore}
            composerRef={composerRef}
            annotations={noAnnotations}
            inspected={inspect?.inspectedId === block.id}
            onInspectTarget={inspect?.publish}
            blockMenu={blockMenu}
          />
        ) : null}
      </div>
    </BlockFrame>
  )
}

export function PanelBlock({ block, height }: Props) {
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
  return <ReadablePanelBlock block={block} body={body} height={height} />
}

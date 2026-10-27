import type { Selections } from "@/api/keys"
import {
  isDataPanel,
  isRowPanel,
  panelTitle,
  type Dashboard,
  type GridPos,
  type LayoutPanel,
  type RowPanel,
} from "@/spec"

import { ALL } from "../search"
import { interpolateTitle } from "./interpolate-title"

export const maxClones = 50
const columns = 24
const defaultPerRow = 4

export type LayoutItem =
  | {
      kind: "row"
      key: string
      row: RowPanel
      title: string
      collapsed: boolean
      childCount: number
      gridPos: GridPos
    }
  | {
      kind: "panel"
      key: string
      panel: LayoutPanel
      title: string
      selections: Selections
      first: boolean
      gridPos: GridPos
    }
  | { kind: "note"; key: string; text: string; gridPos: GridPos }

export type ExpandInput = {
  spec: Dashboard
  collapsed: Record<string, boolean>
  selections: Selections
  values: (name: string) => string[] | undefined
  editing: boolean
}

type Context = { selections: Selections; suffix: string; first: boolean }
type Emitted = { items: LayoutItem[]; bottom: number }
type Piece = { top: number; bottom: number; emit: (dy: number) => Emitted }
type Repeat = { name: string; values: string[]; total: number }

function stack(pieces: Piece[], base: number): Emitted {
  const sorted = [...pieces].sort((a, b) => a.top - b.top)
  const grown: { bottom: number; extra: number }[] = []
  const items: LayoutItem[] = []
  let bottom = 0
  for (const piece of sorted) {
    const dy = grown.reduce(
      (sum, entry) => (entry.bottom <= piece.top ? sum + entry.extra : sum),
      base
    )
    const emitted = piece.emit(dy)
    items.push(...emitted.items)
    grown.push({
      bottom: piece.bottom,
      extra: emitted.bottom - dy - piece.bottom,
    })
    bottom = Math.max(bottom, emitted.bottom)
  }
  return { items, bottom }
}

function noteFor(key: string, total: number, y: number): LayoutItem {
  return {
    kind: "note",
    key: `${key}__note`,
    text: `Showing the first ${maxClones} of ${total} values.`,
    gridPos: { x: 0, y, w: columns, h: 1 },
  }
}

export function expandLayout(input: ExpandInput): LayoutItem[] {
  const { spec, collapsed, selections, values, editing } = input

  function repeatOf(name: string | undefined, context: Context) {
    if (editing || name === undefined) return undefined
    const selected = context.selections[name]
    if (!selected || selected.length === 0) return undefined
    const all = selected.includes(ALL) ? values(name) : selected
    if (!all || all.length === 0) return undefined
    const repeat: Repeat = {
      name,
      values: all.slice(0, maxClones),
      total: all.length,
    }
    return repeat
  }

  function isCollapsed(row: RowPanel): boolean {
    const initial = row.collapsed ?? false
    return editing ? initial : (collapsed[row.id] ?? initial)
  }

  function panelPiece(panel: LayoutPanel, context: Context): Piece {
    const { gridPos } = panel
    const bottom = gridPos.y + gridPos.h
    const dataPanel = isDataPanel(panel) ? panel : undefined
    const repeat = repeatOf(dataPanel?.repeat, context)
    const key = `${panel.id}${context.suffix}`
    if (!repeat || !dataPanel) {
      return {
        top: gridPos.y,
        bottom,
        emit: (dy) => ({
          items: [
            {
              kind: "panel",
              key,
              panel,
              title: interpolateTitle(panelTitle(panel), context.selections),
              selections: context.selections,
              first: context.first,
              gridPos: { ...gridPos, y: gridPos.y + dy },
            },
          ],
          bottom: bottom + dy,
        }),
      }
    }
    const count = repeat.values.length
    const vertical = dataPanel.repeatDirection === "v"
    const perRow = Math.min(count, dataPanel.maxPerRow ?? defaultPerRow)
    const width = Math.max(1, Math.floor(columns / perRow))
    return {
      top: gridPos.y,
      bottom,
      emit: (dy) => {
        const items: LayoutItem[] = repeat.values.map((value, k) => {
          const cloneSelections = {
            ...context.selections,
            [repeat.name]: [value],
          }
          const position: GridPos = vertical
            ? { ...gridPos, y: gridPos.y + dy + k * gridPos.h }
            : {
                x: (k % perRow) * width,
                y: gridPos.y + dy + Math.floor(k / perRow) * gridPos.h,
                w: width,
                h: gridPos.h,
              }
          return {
            kind: "panel",
            key: k === 0 ? key : `${key}__${k}`,
            panel,
            title: interpolateTitle(panelTitle(panel), cloneSelections),
            selections: cloneSelections,
            first: context.first && k === 0,
            gridPos: position,
          }
        })
        let end = Math.max(...items.map((item) => item.gridPos.y + gridPos.h))
        if (repeat.total > count) {
          items.push(noteFor(key, repeat.total, end))
          end += 1
        }
        return { items, bottom: end }
      },
    }
  }

  function rowBlock(row: RowPanel, context: Context, dy: number): Emitted {
    const hidden = isCollapsed(row)
    const children = row.panels ?? []
    const header: LayoutItem = {
      kind: "row",
      key: `${row.id}${context.suffix}`,
      row,
      title: interpolateTitle(row.title, context.selections),
      collapsed: hidden,
      childCount: children.length,
      gridPos: { x: 0, y: row.gridPos.y + dy, w: columns, h: 1 },
    }
    if (hidden) return { items: [header], bottom: header.gridPos.y + 1 }
    const inner = stack(
      children.map((child) => panelPiece(child, context)),
      dy
    )
    return {
      items: [header, ...inner.items],
      bottom: Math.max(header.gridPos.y + 1, inner.bottom),
    }
  }

  function rowPiece(row: RowPanel, context: Context): Piece {
    const top = row.gridPos.y
    const childBottom = isCollapsed(row)
      ? 0
      : Math.max(
          0,
          ...(row.panels ?? []).map(
            (child) => child.gridPos.y + child.gridPos.h
          )
        )
    const repeat = repeatOf(row.repeat, context)
    return {
      top,
      bottom: Math.max(top + 1, childBottom),
      emit: (dy) => {
        if (!repeat) return rowBlock(row, context, dy)
        const items: LayoutItem[] = []
        let cursor = top + dy
        repeat.values.forEach((value, j) => {
          const block = rowBlock(
            row,
            {
              selections: { ...context.selections, [repeat.name]: [value] },
              suffix: j === 0 ? context.suffix : `${context.suffix}__r${j}`,
              first: context.first && j === 0,
            },
            cursor - top
          )
          items.push(...block.items)
          cursor = block.bottom
        })
        if (repeat.total > repeat.values.length) {
          items.push(
            noteFor(`${row.id}${context.suffix}`, repeat.total, cursor)
          )
          cursor += 1
        }
        return { items, bottom: cursor }
      },
    }
  }

  const context: Context = { selections, suffix: "", first: true }
  return stack(
    spec.panels.map((panel) =>
      isRowPanel(panel) ? rowPiece(panel, context) : panelPiece(panel, context)
    ),
    0
  ).items
}

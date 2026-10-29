import { linter, type Diagnostic } from "@codemirror/lint"
import type { Extension } from "@codemirror/state"
import { ViewPlugin, type EditorView } from "@codemirror/view"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"

import { byteRangeToIndices } from "./byte-offsets"

type LintItem = {
  severity: "error" | "warning"
  message: string
  from: number
}

export type LintResult =
  | { status: "ok"; query: string; items: LintItem[] }
  | { status: "unavailable"; query: string; message: string }

const unavailableMessage = "Lint is unavailable right now."
const tooLongMessage = "This query is too long to lint."

function visibleRange(
  length: number,
  { from, to }: { from: number; to: number }
) {
  if (from !== to) return { from, to }
  if (to < length) return { from, to: to + 1 }
  return { from: Math.max(0, from - 1), to }
}

function failureMessage(error: unknown) {
  if (
    error instanceof ApiError &&
    (error.code === "query_too_long" || error.status === 413)
  ) {
    return tooLongMessage
  }
  return unavailableMessage
}

export function serverLinter(
  uid: string,
  onResult: (result: LintResult) => void
): Extension {
  let pending: AbortController | undefined

  async function source(view: EditorView): Promise<Diagnostic[]> {
    pending?.abort()
    const query = view.state.doc.toString()
    if (query.trim() === "") {
      pending = undefined
      onResult({ status: "ok", query, items: [] })
      return []
    }
    const controller = new AbortController()
    pending = controller
    const stale = () =>
      controller.signal.aborted || view.state.doc.toString() !== query
    try {
      const body = await api
        .POST("/datasources/{uid}/lint", {
          params: { path: { uid } },
          body: { query },
          signal: controller.signal,
        })
        .then(unwrap)
      if (stale()) return []
      const ranges = body.diagnostics.map((item) => ({
        item,
        range: byteRangeToIndices(query, item.start, item.end),
      }))
      onResult({
        status: "ok",
        query,
        items: ranges.map(({ item, range }) => ({
          severity: item.severity,
          message: item.message,
          from: range.from,
        })),
      })
      return ranges.map(({ item, range }) => ({
        ...visibleRange(query.length, range),
        severity: item.severity,
        message: item.message,
        source: "lint",
      }))
    } catch (error) {
      if (stale()) return []
      onResult({ status: "unavailable", query, message: failureMessage(error) })
      return []
    }
  }

  return [
    linter(source, { delay: 500 }),
    ViewPlugin.fromClass(
      class {
        destroy() {
          pending?.abort()
        }
      }
    ),
  ]
}

import { linter, type Diagnostic } from "@codemirror/lint"
import type { Extension } from "@codemirror/state"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"

import { byteRangeToIndices } from "./byte-offsets"

export type LintResult =
  | { status: "ok"; query: string; diagnostics: Diagnostic[] }
  | { status: "unavailable" }

function visibleRange(
  length: number,
  { from, to }: { from: number; to: number }
) {
  if (from !== to) return { from, to }
  if (to < length) return { from, to: to + 1 }
  return { from: Math.max(0, from - 1), to }
}

export function serverLinter(
  uid: string,
  onResult: (result: LintResult) => void
): Extension {
  let pending: AbortController | undefined
  return linter(
    async (view) => {
      pending?.abort()
      const query = view.state.doc.toString()
      if (query.trim() === "") {
        pending = undefined
        onResult({ status: "ok", query, diagnostics: [] })
        return []
      }
      const controller = new AbortController()
      pending = controller
      try {
        const body = await api
          .POST("/datasources/{uid}/lint", {
            params: { path: { uid } },
            body: { query },
            signal: controller.signal,
          })
          .then(unwrap)
        if (controller.signal.aborted) return []
        const diagnostics = body.diagnostics.map((item) => ({
          ...visibleRange(
            query.length,
            byteRangeToIndices(query, item.start, item.end)
          ),
          severity: item.severity,
          message: item.message,
          source: "lint",
        }))
        onResult({ status: "ok", query, diagnostics })
        return diagnostics
      } catch (error) {
        if (controller.signal.aborted) return []
        if (error instanceof DOMException && error.name === "AbortError") {
          return []
        }
        onResult({ status: "unavailable" })
        return []
      }
    },
    { delay: 500 }
  )
}

import { api } from "@/api/client"
import type { AuditWindowKey } from "@/api/keys"
import { downloadFile } from "@/dashboards/export-spec"

import { unwrapAudit } from "./query-problem"

export type ExportFormat = "ndjson" | "csv"

const exportTypes: Record<ExportFormat, string> = {
  ndjson: "application/x-ndjson",
  csv: "text/csv",
}

export class RowCounter {
  private readonly decoder = new TextDecoder()
  private readonly format: ExportFormat
  private lines = 0
  private inQuotes = false

  constructor(format: ExportFormat) {
    this.format = format
  }

  push(chunk: Uint8Array) {
    this.count(this.decoder.decode(chunk, { stream: true }))
  }

  private count(text: string) {
    for (const char of text) {
      if (this.format === "csv" && char === '"') {
        this.inQuotes = !this.inQuotes
      } else if (char === "\n" && !this.inQuotes) {
        this.lines++
      }
    }
  }

  rows(): number {
    return this.format === "csv" ? Math.max(0, this.lines - 1) : this.lines
  }
}

type Chunk = Uint8Array<ArrayBuffer>

export type ReadExport = { chunks: Chunk[]; rows: number; cutOff: boolean }

export async function readExport(
  body: ReadableStream<Chunk>,
  format: ExportFormat
): Promise<ReadExport> {
  const counter = new RowCounter(format)
  const chunks: Chunk[] = []
  const reader = body.getReader()
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
      counter.push(value)
    }
  } catch {
    return { chunks, rows: counter.rows(), cutOff: true }
  }
  return { chunks, rows: counter.rows(), cutOff: false }
}

const quotedName = /filename="([^"/\\]+)"/

export function exportFileName(
  disposition: string | null,
  format: ExportFormat,
  now: Date,
  incomplete: boolean
): string {
  const named = disposition?.match(quotedName)?.[1]
  const base =
    named ?? `audit-${now.toISOString().replace(/[:.]/g, "-")}.${format}`
  if (!incomplete) return base
  const dot = base.lastIndexOf(".")
  return dot > 0
    ? `${base.slice(0, dot)}-incomplete${base.slice(dot)}`
    : `${base}-incomplete`
}

export type ExportOutcome =
  | { kind: "complete"; rows: number }
  | { kind: "limit"; rows: number; message: string }
  | { kind: "incomplete"; rows: number; message: string }

function events(count: number): string {
  return `${count.toLocaleString("en-US")} ${count === 1 ? "event" : "events"}`
}

export function exportOutcome(
  read: Pick<ReadExport, "rows" | "cutOff">,
  rowLimit: number | null,
  fileName: string
): ExportOutcome {
  if (read.cutOff) {
    return {
      kind: "incomplete",
      rows: read.rows,
      message: `The download was cut off, so ${fileName} is incomplete: it holds only the first ${events(read.rows)}. Export again to get the whole file.`,
    }
  }
  if (rowLimit !== null && read.rows >= rowLimit) {
    return {
      kind: "limit",
      rows: read.rows,
      message: `The file stops at the export limit of ${events(rowLimit)}, and more events matched. Narrow the time range to export the rest.`,
    }
  }
  return { kind: "complete", rows: read.rows }
}

export function rowLimitOf(header: string | null): number | null {
  const limit = Number(header)
  return header !== null && Number.isInteger(limit) && limit > 0 ? limit : null
}

export async function downloadAuditExport(
  window: AuditWindowKey,
  format: ExportFormat,
  signal?: AbortSignal
): Promise<ExportOutcome> {
  const result = await api.GET("/audit/export", {
    params: {
      query: {
        q: window.q === "" ? undefined : window.q,
        since: window.since,
        until: window.until,
        format,
      },
    },
    parseAs: "stream",
    signal,
  })
  const body = unwrapAudit(result)
  if (body === null) throw new TypeError("The export answered without a body.")
  const read = await readExport(body, format)
  const fileName = exportFileName(
    result.response.headers.get("content-disposition"),
    format,
    new Date(),
    read.cutOff
  )
  downloadFile(new Blob(read.chunks, { type: exportTypes[format] }), fileName)
  return exportOutcome(
    read,
    rowLimitOf(result.response.headers.get("x-export-row-limit")),
    fileName
  )
}

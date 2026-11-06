import { Link } from "@tanstack/react-router"

import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { traceLink } from "@/lib/links"
import { zonedDateTimeMs } from "@/lib/time"

import {
  countText,
  durationText,
  hasErrorCounts,
  hasSpanCounts,
  type TraceRow,
} from "./trace-rows"

function TraceStatus({ errors }: { errors: number }) {
  return errors > 0 ? (
    <StatusLabel tone="critical">
      Failed · {countText(errors, "error")}
    </StatusLabel>
  ) : (
    <StatusLabel tone="ok">OK</StatusLabel>
  )
}

export function TraceTable({
  rows,
  datasourceUid,
  range,
}: {
  rows: TraceRow[]
  datasourceUid: string
  range: { from: string; to: string }
}) {
  const spans = hasSpanCounts(rows)
  const status = hasErrorCounts(rows)
  const slowest = Math.max(0, ...rows.map((row) => row.durationMs ?? 0))
  return (
    <TableCard label="Traces" stackOnNarrow minWidth={760}>
      <thead>
        <tr>
          <HeaderCell>Started</HeaderCell>
          <HeaderCell>Service</HeaderCell>
          <HeaderCell>Span</HeaderCell>
          <HeaderCell className="text-right">Duration</HeaderCell>
          {spans ? <HeaderCell className="text-right">Spans</HeaderCell> : null}
          {status ? <HeaderCell>Status</HeaderCell> : null}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.traceId}>
            <Cell className="whitespace-nowrap">
              <Link
                {...traceLink(row.traceId, datasourceUid, range)}
                aria-label={`Open trace ${row.traceId}`}
                className="font-medium tabular-nums underline-offset-3 hover:underline"
              >
                {row.start === null ? (
                  <span className="font-mono">{row.traceId.slice(0, 8)}</span>
                ) : (
                  zonedDateTimeMs(row.start, undefined)
                )}
              </Link>
            </Cell>
            <Cell>{row.service || "-"}</Cell>
            <Cell className="max-w-80 truncate">{row.span || "-"}</Cell>
            <Cell className="text-right whitespace-nowrap tabular-nums">
              {durationText(row.durationMs)}
              <div
                aria-hidden="true"
                className="mt-1 h-0.5 rounded-full bg-series-1"
                style={{
                  width:
                    row.durationMs !== null && slowest > 0
                      ? `${(row.durationMs / slowest) * 100}%`
                      : 0,
                }}
              />
            </Cell>
            {spans ? (
              <Cell className="text-right tabular-nums">
                {row.spans === null ? "-" : row.spans.toLocaleString("en")}
              </Cell>
            ) : null}
            {status && row.errors !== null ? (
              <Cell className="whitespace-nowrap">
                <TraceStatus errors={row.errors} />
              </Cell>
            ) : null}
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}

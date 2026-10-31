import { useId, useState } from "react"
import { ChevronRightIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { cn } from "@/lib/utils"

import {
  blockHeading,
  blockStatus,
  blockTally,
  implicitText,
  omittedText,
  outcomeLabel,
  specText,
  splitOptions,
  tallyText,
  type ImportBlock,
  type ImportOptionLine,
} from "./blocks"
import type { ImportReport } from "./report"

const toggleClass =
  "flex w-full items-start gap-2 rounded-sm text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"

function Chevron({ open }: { open: boolean }) {
  return (
    <ChevronRightIcon
      aria-hidden
      className={cn(
        "mt-0.5 size-3.5 flex-none text-muted-foreground motion-safe:transition-transform",
        open && "rotate-90"
      )}
    />
  )
}

function OptionLines({ lines }: { lines: ImportOptionLine[] }) {
  return (
    <ul>
      {lines.map((line, index) => {
        const label = outcomeLabel(line.outcome)
        const spec = specText(line)
        return (
          <li
            key={`${line.path}:${index}`}
            className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 py-1"
          >
            <span className="font-mono text-[11px] wrap-anywhere">
              {line.path}
            </span>
            <span className="text-xs">
              <StatusLabel tone={label.tone}>{label.text}</StatusLabel>
            </span>
            {spec ? (
              <span
                className={cn(
                  "wrap-anywhere",
                  line.outcome === "mapped"
                    ? "font-mono text-[11px]"
                    : "text-xs text-muted-foreground"
                )}
              >
                {spec}
              </span>
            ) : null}
          </li>
        )
      })}
    </ul>
  )
}

function ImplicitLines({ block }: { block: ImportBlock }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const text = implicitText(block)
  const lines = splitOptions(block).implicit
  if (text === null) return null
  if (lines.length === 0) {
    return (
      <span className="block py-1 text-xs text-muted-foreground">{text}</span>
    )
  }
  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        className={cn(toggleClass, "py-1 text-xs text-muted-foreground")}
        onClick={() => setOpen((value) => !value)}
      >
        <Chevron open={open} />
        {text}
      </button>
      {open ? (
        <div id={id} className="pl-5">
          <OptionLines lines={lines} />
        </div>
      ) : null}
    </div>
  )
}

function BlockBody({
  block,
  report,
  id,
}: {
  block: ImportBlock
  report: ImportReport
  id: string
}) {
  const omitted = omittedText(block)
  return (
    <div id={id} className="pt-1 pl-5">
      <OptionLines lines={splitOptions(block).main} />
      <ImplicitLines block={block} />
      {omitted ? (
        <p className="py-1 text-xs text-muted-foreground">{omitted}</p>
      ) : null}
      {block.blocks.length > 0 ? (
        <ul>
          {block.blocks.map((child, index) => (
            <BlockItem
              key={`${child.kind}:${child.path}:${index}`}
              block={child}
              report={report}
              depth={1}
            />
          ))}
        </ul>
      ) : null}
    </div>
  )
}

function BlockItem({
  block,
  report,
  depth,
}: {
  block: ImportBlock
  report: ImportReport
  depth: number
}) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const heading = blockHeading(block)
  const status = blockStatus(block, report)
  const itemClass = depth === 0 ? "border-b py-2.5 last:border-b-0" : "py-1.5"

  const title = (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span>
        <span className="text-[13px] font-medium wrap-anywhere">
          {heading.title}
        </span>
        {heading.subtitle ? (
          <span className="ml-1 text-xs text-muted-foreground">
            {heading.subtitle}
          </span>
        ) : null}
      </span>
      {block.outcome !== null && block.reason ? (
        <span className="text-xs text-muted-foreground">{block.reason}</span>
      ) : null}
    </span>
  )

  if (block.outcome !== null) {
    const label = outcomeLabel(block.outcome)
    return (
      <li className={itemClass}>
        <div className="flex items-start gap-2">
          <span className="size-3.5 flex-none" />
          {title}
          <span className="text-xs">
            <StatusLabel tone={label.tone}>{label.text}</StatusLabel>
          </span>
        </div>
      </li>
    )
  }

  return (
    <li className={itemClass}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        className={toggleClass}
        onClick={() => setOpen((value) => !value)}
      >
        <Chevron open={open} />
        {title}
        {status ? (
          <span className="text-xs">
            <StatusLabel tone={status.tone}>{status.text}</StatusLabel>
          </span>
        ) : null}
        <span className="text-xs text-muted-foreground">
          {tallyText(blockTally(block))}
        </span>
      </button>
      {open ? <BlockBody block={block} report={report} id={id} /> : null}
    </li>
  )
}

export function BlockList({ report }: { report: ImportReport }) {
  return (
    <ul>
      {(report.blocks ?? []).map((block, index) => (
        <BlockItem
          key={`${block.kind}:${block.path}:${index}`}
          block={block}
          report={report}
          depth={0}
        />
      ))}
    </ul>
  )
}

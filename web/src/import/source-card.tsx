import { useRef, useState, type ChangeEvent } from "react"
import { CircleAlertIcon, FileJsonIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

import type { Failure } from "./errors"
import type { Source } from "./queries"

type Props = {
  source: Source | null
  hint: string
  dragging: boolean
  error: Failure | null
  onFile: (file: File) => void
  onText: (text: string) => void
  onRetry: () => void
}

function FileTile() {
  return (
    <span
      aria-hidden="true"
      className="grid size-9 flex-none place-items-center rounded-lg border"
    >
      <FileJsonIcon className="size-[18px]" />
    </span>
  )
}

export function SourceCard({
  source,
  hint,
  dragging,
  error,
  onFile,
  onText,
  onRetry,
}: Props) {
  const input = useRef<HTMLInputElement>(null)
  const [pasting, setPasting] = useState(false)
  const [draft, setDraft] = useState("")

  const choose = () => input.current?.click()
  const onChosen = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (file) onFile(file)
  }
  const closePaste = () => {
    setPasting(false)
    setDraft("")
  }

  let body
  if (source) {
    body = (
      <>
        <div className="flex flex-wrap items-center gap-3 p-3.5">
          <FileTile />
          <span className="flex min-w-[180px] flex-1 flex-col gap-0.5">
            <span
              className={cn(
                "break-all",
                source.pasted ? "text-[13px]" : "font-mono text-[13px]"
              )}
            >
              {source.name}
            </span>
            <span className="text-xs text-muted-foreground">{hint}</span>
          </span>
          <Button variant="outline" className="font-normal" onClick={choose}>
            Choose another file
          </Button>
        </div>
        <p className="px-3.5 pb-3.5 text-xs text-muted-foreground">
          You can also paste JSON or drop a file anywhere on this page.
        </p>
      </>
    )
  } else if (pasting) {
    body = (
      <div className="flex flex-col gap-3 p-3.5">
        <Textarea
          autoFocus
          rows={12}
          value={draft}
          aria-label="Grafana dashboard JSON"
          placeholder='{ "title": ... }'
          className="font-mono text-xs"
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.stopPropagation()
              closePaste()
            }
          }}
        />
        <div className="flex gap-2">
          <Button
            disabled={draft.trim() === ""}
            onClick={() => {
              onText(draft)
              closePaste()
            }}
          >
            Preview
          </Button>
          <Button
            variant="outline"
            className="font-normal"
            onClick={closePaste}
          >
            Cancel
          </Button>
        </div>
      </div>
    )
  } else {
    body = (
      <div className="flex flex-col items-center gap-3 px-3.5 py-8 text-center">
        <FileTile />
        <p className="text-sm">Drop a Grafana dashboard JSON file here</p>
        <div className="flex flex-wrap justify-center gap-2">
          <Button variant="outline" className="font-normal" onClick={choose}>
            Choose file
          </Button>
          <Button
            variant="outline"
            className="font-normal"
            onClick={() => setPasting(true)}
          >
            Paste JSON
          </Button>
        </div>
      </div>
    )
  }

  return (
    <section
      aria-labelledby="import-source"
      data-dragging={dragging || undefined}
      className={cn(
        "rounded-card border bg-background",
        dragging && "border-foreground"
      )}
    >
      <div className="border-b px-3.5 py-3">
        <h2 id="import-source" className="text-sm font-semibold">
          Dashboard JSON
        </h2>
      </div>
      <input
        ref={input}
        type="file"
        accept=".json,application/json"
        aria-label="Grafana dashboard file"
        className="sr-only"
        tabIndex={-1}
        onChange={onChosen}
      />
      {body}
      {error ? (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 pb-3.5">
          <p role="alert" className="flex items-start gap-2 text-[13px]">
            <CircleAlertIcon
              aria-hidden="true"
              className="mt-0.5 size-3.5 flex-none text-status-critical"
            />
            {error.message}
          </p>
          {error.retry ? (
            <Button variant="outline" size="sm" onClick={onRetry}>
              Try again
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

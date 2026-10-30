import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { automaticFolder } from "./folder-choice"

type Props = {
  choices: readonly string[]
  value: string
  failed: boolean
  disabled: boolean
  onChange: (title: string) => void
}

const headingId = "import-folder"

export function FolderCard({
  choices,
  value,
  failed,
  disabled,
  onChange,
}: Props) {
  const items = [
    { value: automaticFolder, label: "Automatic" },
    ...choices.map((title) => ({ value: title, label: title })),
  ]
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-card border bg-background"
    >
      <div className="border-b px-3.5 py-3">
        <h2 id={headingId} className="text-sm font-semibold">
          Folder
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground">
          Automatic uses the folder named in the Grafana export, else the folder
          you last created a dashboard in, else General.
        </p>
      </div>
      <div className="flex flex-col gap-2 p-3.5">
        <Select
          items={items}
          value={value}
          disabled={disabled}
          onValueChange={(title) => onChange(title ?? automaticFolder)}
        >
          <SelectTrigger
            aria-labelledby={headingId}
            className="w-full max-w-60"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {items.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {failed ? (
          <p role="alert" className="text-xs text-muted-foreground">
            Could not load your folders, so only Automatic is offered.
          </p>
        ) : null}
      </div>
    </section>
  )
}

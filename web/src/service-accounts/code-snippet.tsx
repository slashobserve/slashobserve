import { CopyButton } from "@/components/copy-button"
import { cn } from "@/lib/utils"

type Props = { code: string; label: string; wrap: boolean }

export function CodeSnippet({ code, label, wrap }: Props) {
  return (
    <div className="relative rounded-lg border bg-muted/50">
      <pre
        tabIndex={0}
        role="region"
        aria-label={label}
        className={cn(
          "rounded-lg p-3 pr-11 font-mono text-xs leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring",
          wrap
            ? "wrap-anywhere whitespace-pre-wrap"
            : "overflow-x-auto whitespace-pre"
        )}
      >
        <code>{code}</code>
      </pre>
      <div className="absolute top-1.5 right-1.5">
        <CopyButton
          text={code}
          label={`Copy ${label}`}
          variant="ghost"
          size="icon-sm"
        />
      </div>
    </div>
  )
}
